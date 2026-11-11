package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultEndpoint = "https://api.opper.ai/v3/compat/v1/systemone"
	DefaultModel    = "melious/clef"
	choiceQuestion  = "topic"
	choiceGuidance  = "Pick the option that best describes what the text is asking about."
	noulPrefix      = "q_"
	maxResponseSize = 64 << 10

	DefaultCompletionEndpoint = "https://api.opper.ai/v3/compat/chat/completions"
	DefaultCompletionModel    = "mistral/mistral-small-2603"
	maxCompletionTokens       = 220

	DefaultSpeechEndpoint = "https://api.opper.ai/v3/audio/speech"
	DefaultSpeechModel    = "mistral/voxtral-mini-tts-3-0"
	maxAudioSize          = 4 << 20
)

type Client struct {
	HTTP     *http.Client
	endpoint string
	model    string
	apiKey   string
}

func NewClient(apiKey string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 3 * time.Second
	return &Client{
		HTTP:     &http.Client{Transport: transport},
		endpoint: DefaultEndpoint,
		model:    DefaultModel,
		apiKey:   apiKey,
	}
}

type question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type request struct {
	Model     string              `json:"model"`
	State     string              `json:"state"`
	Questions map[string]question `json:"questions"`
}

type answer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Noul          *float64           `json:"noul"`
}

type response struct {
	Answers map[string]answer `json:"answers"`
}

type Evaluation struct {
	Topics map[string]float64
	Scores map[string]float64
}

func (c *Client) Evaluate(ctx context.Context, state string, choice []Option, nouls []Option) (Evaluation, error) {
	qs := make(map[string]question, len(nouls)+1)
	if len(choice) > 0 {
		criteria := make(map[string]string, len(choice))
		for _, o := range choice {
			criteria[o.ID] = o.Text
		}
		qs[choiceQuestion] = question{Type: "choice", Instructions: choiceGuidance, Criteria: criteria}
	}
	for _, q := range nouls {
		qs[noulPrefix+q.ID] = question{Type: "noul", Instructions: q.Text}
	}
	res, err := c.call(ctx, request{Model: c.model, State: state, Questions: qs})
	if err != nil {
		return Evaluation{}, err
	}

	var ev Evaluation
	if len(choice) > 0 {
		a, ok := res.Answers[choiceQuestion]
		if !ok || len(a.Probabilities) == 0 {
			return Evaluation{}, fmt.Errorf("model response has no choice")
		}
		ev.Topics = a.Probabilities
	}
	ev.Scores = make(map[string]float64, len(nouls))
	for _, q := range nouls {
		a, ok := res.Answers[noulPrefix+q.ID]
		if !ok || a.Noul == nil {
			return Evaluation{}, fmt.Errorf("model response has no answer for %q", q.ID)
		}
		ev.Scores[q.ID] = *a.Noul
	}
	return ev, nil
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type completionResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	var res completionResponse
	err := c.post(ctx, DefaultCompletionEndpoint, "portfolio-compose", completionRequest{
		Model:       DefaultCompletionModel,
		Messages:    []message{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Temperature: 0,
		MaxTokens:   maxCompletionTokens,
	}, &res)
	if err != nil {
		return "", err
	}
	if len(res.Choices) == 0 || strings.TrimSpace(res.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("completion is empty")
	}
	return strings.TrimSpace(res.Choices[0].Message.Content), nil
}

func (c *Client) call(ctx context.Context, body request) (response, error) {
	var out response
	err := c.post(ctx, c.endpoint, "portfolio-query", body, &out)
	return out, err
}

func (c *Client) post(ctx context.Context, endpoint, name string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Opper-Name", name)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call model: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("model returned status %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponseSize)).Decode(out); err != nil {
		return fmt.Errorf("decode model response: %w", err)
	}
	return nil
}

type speechRequest struct {
	Model  string `json:"model"`
	Input  string `json:"input"`
	Format string `json:"format"`
}

func (c *Client) Speak(ctx context.Context, text string) ([]byte, error) {
	raw, err := json.Marshal(speechRequest{Model: DefaultSpeechModel, Input: text, Format: "mp3"})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DefaultSpeechEndpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")
	req.Header.Set("X-Opper-Name", "portfolio-speech")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call speech model: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "audio/") {
		return nil, fmt.Errorf("speech model returned status %d (%s)", res.StatusCode, res.Header.Get("Content-Type"))
	}
	return io.ReadAll(io.LimitReader(res.Body, maxAudioSize))
}
