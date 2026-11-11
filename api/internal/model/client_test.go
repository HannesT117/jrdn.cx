package model

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type capturedRequest struct {
	Model     string                    `json:"model"`
	State     string                    `json:"state"`
	Questions map[string]map[string]any `json:"questions"`
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fakeOpper(t *testing.T, status int, body []byte, got *capturedRequest, auth *string) *Client {
	t.Helper()
	c := NewClient("op-test")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got != nil {
			if err := json.NewDecoder(r.Body).Decode(got); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		if auth != nil {
			*auth = r.Header.Get("Authorization")
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})}
	return c
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEvaluateSendsChoiceAndNoulQuestionsInOneCall(t *testing.T) {
	var req capturedRequest
	var auth string
	c := fakeOpper(t, http.StatusOK, fixture(t, "choice.json"), &req, &auth)

	got, err := c.Evaluate(context.Background(), "Do you bake bread?",
		[]Option{{ID: "bread", Text: "Bread"}, {ID: "none", Text: "Nothing fits"}},
		[]Option{{ID: "topic", Text: "Is this fact relevant?"}},
	)
	if err == nil {
		t.Fatalf("expected an error for the missing noul answer, got %+v", got)
	}
	if auth != "Bearer op-test" || req.Model != DefaultModel || req.State != "Do you bake bread?" {
		t.Errorf("got auth %q model %q state %q", auth, req.Model, req.State)
	}
	choice := req.Questions[choiceQuestion]
	criteria, _ := choice["criteria"].(map[string]any)
	if choice["type"] != "choice" || criteria["bread"] != "Bread" || criteria["none"] != "Nothing fits" {
		t.Errorf("got choice question %+v", choice)
	}
	if q := req.Questions["q_topic"]; q["type"] != "noul" || q["instructions"] != "Is this fact relevant?" {
		t.Errorf("a noul id equal to the choice key must not clash, got %+v", req.Questions)
	}
}

func TestEvaluateReturnsThePick(t *testing.T) {
	c := fakeOpper(t, http.StatusOK, fixture(t, "choice.json"), nil, nil)
	got, err := c.Evaluate(context.Background(), "Do you bake bread?", []Option{{ID: "bread", Text: "Bread"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pick.ID != "bread" || got.Pick.P != 0.97697844944979 {
		t.Fatalf("got %+v", got.Pick)
	}
}

func TestEvaluateWithoutChoiceReturnsScores(t *testing.T) {
	var req capturedRequest
	c := fakeOpper(t, http.StatusOK, fixture(t, "noul.json"), &req, nil)
	got, err := c.Evaluate(context.Background(), "Birthday cake order", nil, []Option{
		{ID: "s_cakes", Text: "Cakes?"},
		{ID: "l_sushi", Text: "Sushi?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Scores["s_cakes"] != 0.940078594487036 || got.Scores["l_sushi"] != 0.6397360587186955 {
		t.Fatalf("got %v", got.Scores)
	}
	if _, ok := req.Questions[choiceQuestion]; ok || len(req.Questions) != 2 {
		t.Errorf("got questions %+v", req.Questions)
	}
}

func TestUpstreamFailuresBecomeErrors(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"payment required": {http.StatusPaymentRequired, `{"error":"budget"}`},
		"server error":     {http.StatusBadGateway, ``},
		"malformed body":   {http.StatusOK, `not json`},
		"missing answer":   {http.StatusOK, `{"answers":{}}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := fakeOpper(t, tc.status, []byte(tc.body), nil, nil)
			if _, err := c.Evaluate(context.Background(), "q", []Option{{ID: "a", Text: "a"}}, nil); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCompleteSendsSystemAndUserMessages(t *testing.T) {
	var got struct {
		Model       string  `json:"model"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
		Messages    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	var url string
	c := NewClient("op-test")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		url = r.URL.String()
		_ = json.NewDecoder(r.Body).Decode(&got)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"  Sourdough daily.  "}}]}`)),
		}, nil
	})}

	text, err := c.Complete(context.Background(), "Be brief.", "Facts: sourdough")
	if err != nil {
		t.Fatal(err)
	}
	if text != "Sourdough daily." {
		t.Fatalf("got %q", text)
	}
	if url != DefaultCompletionEndpoint || got.Model != DefaultCompletionModel || got.Temperature != 0 || got.MaxTokens == 0 {
		t.Errorf("got url %q body %+v", url, got)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[0].Content != "Be brief." || got.Messages[1].Role != "user" {
		t.Errorf("got messages %+v", got.Messages)
	}
}

func TestCompleteRejectsEmptyOrFailedResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"error status": {http.StatusInternalServerError, `{}`},
		"no choices":   {http.StatusOK, `{"choices":[]}`},
		"empty text":   {http.StatusOK, `{"choices":[{"message":{"content":"   "}}]}`},
	} {
		t.Run(name, func(t *testing.T) {
			c := fakeOpper(t, tc.status, []byte(tc.body), nil, nil)
			if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestEvaluateRespectsContextDeadline(t *testing.T) {
	c := NewClient("op-test")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(2 * time.Second):
			return nil, nil
		}
	})}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Evaluate(ctx, "q", []Option{{ID: "a", Text: "a"}}, nil); err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > time.Second {
		t.Fatal("call did not stop at the deadline")
	}
}

func TestSpeakRequestsMP3AndReturnsTheAudio(t *testing.T) {
	var got map[string]any
	var accept, url string
	c := NewClient("op-test")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		url, accept = r.URL.String(), r.Header.Get("Accept")
		_ = json.NewDecoder(r.Body).Decode(&got)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"audio/mpeg"}},
			Body:       io.NopCloser(bytes.NewReader([]byte("ID3fake"))),
		}, nil
	})}

	audio, err := c.Speak(context.Background(), "Fresh sourdough every morning.")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "ID3fake" {
		t.Fatalf("got %q", audio)
	}
	if url != DefaultSpeechEndpoint || accept != "audio/mpeg" {
		t.Errorf("got url %q accept %q", url, accept)
	}
	if got["model"] != DefaultSpeechModel || got["input"] != "Fresh sourdough every morning." || got["format"] != "mp3" {
		t.Errorf("got body %v", got)
	}
}

func TestSpeakRejectsNonAudioResponses(t *testing.T) {
	cases := map[string]struct {
		status      int
		contentType string
	}{
		"error status": {http.StatusBadRequest, "audio/mpeg"},
		"json body":    {http.StatusOK, "application/json"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := NewClient("op-test")
			c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tc.status,
					Header:     http.Header{"Content-Type": {tc.contentType}},
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{}}`))),
				}, nil
			})}
			if _, err := c.Speak(context.Background(), "hi"); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
