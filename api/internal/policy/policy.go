package policy

import (
	"cmp"
	"slices"

	"github.com/HannesT117/johannestrepesch.de/api/internal/content"
	"github.com/HannesT117/johannestrepesch.de/api/internal/model"
)

const (
	FitThreshold   = 0.6
	GuardThreshold = 0.5
	StrongMatches  = 3
)

const (
	KindAnswer      = "answer"
	KindNoAnswer    = "no_answer"
	KindFit         = "fit"
	KindLetsTalk    = "lets_talk"
	KindUnavailable = "unavailable"
	KindError       = "error"
)

type Response struct {
	Kind     string   `json:"kind"`
	ID       string   `json:"id,omitempty"`
	Text     string   `json:"text,omitempty"`
	Matches  []string `json:"matches,omitempty"`
	Learning []string `json:"learning,omitempty"`
	Strong   bool     `json:"strong,omitempty"`
}

func FitQuestions(c content.Content) []model.Option {
	qs := make([]model.Option, 0, len(c.Strengths)+len(c.Learning)+1)
	for _, cr := range slices.Concat(c.Strengths, c.Learning) {
		qs = append(qs, model.Option{ID: cr.ID, Text: cr.Question})
	}
	return append(qs, model.Option{ID: content.GuardID, Text: c.GuardQuestion})
}

func Fit(c content.Content, scores map[string]float64) Response {
	if scores[content.GuardID] >= GuardThreshold {
		return Response{Kind: KindLetsTalk}
	}
	var passing []content.Criterion
	for _, s := range c.Strengths {
		if scores[s.ID] >= FitThreshold {
			passing = append(passing, s)
		}
	}
	if len(passing) == 0 {
		return Response{Kind: KindLetsTalk}
	}
	slices.SortStableFunc(passing, func(a, b content.Criterion) int { return cmp.Compare(scores[b.ID], scores[a.ID]) })
	matches := make([]string, len(passing))
	for i, s := range passing {
		matches[i] = s.Label
	}

	var learning []string
	for _, l := range c.Learning {
		if scores[l.ID] >= FitThreshold {
			learning = append(learning, l.Label)
		}
	}
	return Response{Kind: KindFit, Matches: matches, Learning: learning, Strong: len(matches) >= StrongMatches}
}
