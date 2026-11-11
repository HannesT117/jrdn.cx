package httpapi

import (
	"context"
	_ "embed"
	"encoding/json"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/HannesT117/johannestrepesch.de/api/internal/content"
	"github.com/HannesT117/johannestrepesch.de/api/internal/limit"
	"github.com/HannesT117/johannestrepesch.de/api/internal/model"
	"github.com/HannesT117/johannestrepesch.de/api/internal/policy"
)

const (
	MaxBodyBytes = 32 << 10
	ModelTimeout = 10 * time.Second
)

//go:embed limits.json
var limitsJSON []byte

var MaxChars = func() map[string]int {
	var m map[string]int
	if err := json.Unmarshal(limitsJSON, &m); err != nil {
		panic(err)
	}
	return m
}()

type Config struct {
	Enabled        bool
	Origins        []string
	ClientIPHeader string
}

type Model interface {
	Evaluate(ctx context.Context, state string, choice, nouls []model.Option) (model.Evaluation, error)
	Complete(ctx context.Context, system, user string) (string, error)
	Speak(ctx context.Context, text string) ([]byte, error)
}

type Budget interface {
	Allow(ctx context.Context, now time.Time) error
}

type handler struct {
	cfg     Config
	content content.Content
	model   Model
	limiter *limit.Limiter
	budget  Budget

	textMu sync.Mutex
	texts  map[string]string

	audioMu sync.Mutex
	audio   map[string][]byte
}

type query struct {
	Mode    string `json:"mode"`
	Text    string `json:"text"`
	Website string `json:"website"`
}

func New(cfg Config, c content.Content, m Model, l *limit.Limiter, b Budget) http.Handler {
	mux := http.NewServeMux()
	h := &handler{cfg: cfg, content: c, model: m, limiter: l, budget: b, texts: map[string]string{}, audio: map[string][]byte{}}
	mux.HandleFunc("POST /api/query", h.query)
	mux.HandleFunc("GET /api/speech/{id}", h.speech)
	return mux
}

func (h *handler) query(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if !h.cfg.Enabled {
		reply(w, http.StatusServiceUnavailable, policy.KindUnavailable)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !slices.Contains(h.cfg.Origins, origin) {
		reply(w, http.StatusForbidden, policy.KindError)
		return
	}
	q, ok := decode(w, r)
	if !ok {
		reply(w, http.StatusBadRequest, policy.KindError)
		return
	}
	if q.Website != "" {
		reply(w, http.StatusOK, policy.KindNoAnswer)
		return
	}
	if !h.withinBudget(r.Context()) {
		reply(w, http.StatusServiceUnavailable, policy.KindUnavailable)
		return
	}
	if err := h.limiter.Consume(h.clientIP(r), time.Now()); err != nil {
		reply(w, http.StatusTooManyRequests, policy.KindUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), ModelTimeout)
	defer cancel()
	res, err := h.answer(ctx, q)
	if err != nil {
		slog.Error("model call failed", "mode", q.Mode, "err", err)
		reply(w, http.StatusServiceUnavailable, policy.KindUnavailable)
		return
	}
	write(w, http.StatusOK, res)
}

func (h *handler) answer(ctx context.Context, q query) (policy.Response, error) {
	if q.Mode == "fit" {
		ev, err := h.model.Evaluate(ctx, q.Text, nil, policy.FitQuestions(h.content))
		return policy.Fit(h.content, ev.Scores), err
	}
	choice, nouls := policy.AskQuestions(h.content)
	ev, err := h.model.Evaluate(ctx, q.Text, choice, nouls)
	if err != nil {
		return policy.Response{}, err
	}
	sel, ok := policy.Select(h.content, ev)
	if !ok {
		return policy.Response{Kind: policy.KindNoAnswer}, nil
	}
	return policy.Answer(sel.Key(), h.compose(ctx, sel)), nil
}

func (h *handler) compose(ctx context.Context, sel policy.Selection) string {
	key := sel.Key()
	h.textMu.Lock()
	defer h.textMu.Unlock()
	if text, ok := h.texts[key]; ok {
		return text
	}
	system, user := policy.Prompt(h.content, sel)
	draft, err := h.model.Complete(ctx, system, user)
	if err != nil {
		slog.Error("compose call failed", "key", key, "err", err)
		return policy.Verify(h.content, sel, "")
	}
	text := policy.Verify(h.content, sel, draft)
	if text != draft {
		slog.Warn("composed answer rejected, using facts", "key", key, "draft", draft)
	}
	h.texts[key] = text
	return text
}

func decode(w http.ResponseWriter, r *http.Request) (query, bool) {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		return query{}, false
	}
	var q query
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err := dec.Decode(&q); err != nil {
		return query{}, false
	}
	q.Text = strings.TrimSpace(q.Text)
	maxChars, known := MaxChars[q.Mode]
	if !known || q.Text == "" || utf8.RuneCountInString(q.Text) > maxChars {
		return query{}, false
	}
	return q, true
}

func (h *handler) clientIP(r *http.Request) string {
	if h.cfg.ClientIPHeader != "" {
		if v := r.Header.Get(h.cfg.ClientIPHeader); v != "" {
			hops := strings.Split(v, ",")
			return strings.TrimSpace(hops[len(hops)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func reply(w http.ResponseWriter, status int, kind string) {
	write(w, status, policy.Response{Kind: kind})
}

func write(w http.ResponseWriter, status int, res policy.Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(res)
}

func (h *handler) withinBudget(ctx context.Context) bool {
	if h.budget == nil {
		return true
	}
	if err := h.budget.Allow(ctx, time.Now()); err != nil {
		slog.Warn("query box resting", "err", err)
		return false
	}
	return true
}

func (h *handler) speech(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !h.cfg.Enabled {
		reply(w, http.StatusServiceUnavailable, policy.KindUnavailable)
		return
	}
	id := r.PathValue("id")
	sel, ok := policy.ParseKey(h.content, id)
	if !ok {
		reply(w, http.StatusNotFound, policy.KindError)
		return
	}

	h.audioMu.Lock()
	defer h.audioMu.Unlock()
	audio, cached := h.audio[id]
	if !cached {
		if !h.withinBudget(r.Context()) {
			reply(w, http.StatusServiceUnavailable, policy.KindUnavailable)
			return
		}
		if err := h.limiter.Consume(h.clientIP(r), time.Now()); err != nil {
			reply(w, http.StatusTooManyRequests, policy.KindUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), ModelTimeout)
		defer cancel()
		var err error
		if audio, err = h.model.Speak(ctx, h.compose(ctx, sel)); err != nil {
			slog.Error("speech call failed", "id", id, "err", err)
			reply(w, http.StatusServiceUnavailable, policy.KindUnavailable)
			return
		}
		h.audio[id] = audio
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(audio)
}
