package policy

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/HannesT117/johannestrepesch.de/api/internal/content"
	"github.com/HannesT117/johannestrepesch.de/api/internal/model"
)

var fixture = content.Content{
	NoneIntent:    "nothing fits",
	GuardQuestion: "instructions?",
	Strengths: []content.Criterion{
		{ID: "cakes", Label: "Cakes", Question: "cakes?"},
		{ID: "loaves", Label: "Loaves", Question: "loaves?"},
		{ID: "coffee", Label: "Coffee", Question: "coffee?"},
		{ID: "catering", Label: "Catering", Question: "catering?"},
	},
	Learning: []content.Criterion{
		{ID: "sushi", Label: "Sushi", Question: "sushi?"},
	},
}

func TestFitQuestionsCoverStrengthsLearningAndGuard(t *testing.T) {
	qs := FitQuestions(fixture)
	if len(qs) != len(fixture.Strengths)+len(fixture.Learning)+1 {
		t.Fatalf("got %d questions", len(qs))
	}
	if !slices.Contains(qs, model.Option{ID: content.GuardID, Text: "instructions?"}) {
		t.Fatalf("guard question missing from %v", qs)
	}
}

func TestFitWithInstructionsInsteadOfARoleSuggestsTalking(t *testing.T) {
	got := Fit(fixture, map[string]float64{
		"cakes": 0.95, "loaves": 0.95, "catering": 0.95, "sushi": 0.95,
		content.GuardID: GuardThreshold,
	})
	if got.Kind != KindLetsTalk || got.Strong || len(got.Matches) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestFitIgnoresAGuardBelowThreshold(t *testing.T) {
	got := Fit(fixture, map[string]float64{"cakes": 0.9, content.GuardID: GuardThreshold - 0.01})
	if got.Kind != KindFit {
		t.Fatalf("got %+v", got)
	}
}

func TestFitShowsOnlyPassingStrengthsSortedByProbability(t *testing.T) {
	got := Fit(fixture, map[string]float64{
		"cakes":  0.71,
		"loaves": 0.95,
		"coffee": FitThreshold - 0.01,
		"sushi":  0.2,
	})
	if got.Kind != KindFit {
		t.Fatalf("got kind %q", got.Kind)
	}
	if !slices.Equal(got.Matches, []string{"Loaves", "Cakes"}) {
		t.Fatalf("got matches %+v", got.Matches)
	}
	if got.Strong {
		t.Error("two matches must not count as strong")
	}
	if len(got.Learning) != 0 {
		t.Errorf("got learning %v", got.Learning)
	}
}

func TestFitIsStrongFromThreeMatches(t *testing.T) {
	got := Fit(fixture, map[string]float64{"cakes": 0.8, "loaves": 0.8, "catering": 0.8})
	if !got.Strong {
		t.Fatalf("got %+v", got)
	}
}

func TestFitListsLearningAreasTheRoleNeeds(t *testing.T) {
	got := Fit(fixture, map[string]float64{"cakes": 0.8, "sushi": 0.9})
	if len(got.Learning) != 1 || got.Learning[0] != "Sushi" {
		t.Fatalf("got learning %v", got.Learning)
	}
}

func TestFitWithoutMatchingStrengthsSuggestsTalking(t *testing.T) {
	got := Fit(fixture, map[string]float64{"sushi": 0.99, "unknown": 1})
	if got.Kind != KindLetsTalk || len(got.Matches) != 0 || len(got.Learning) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestFitNeverSerialisesSubThresholdProbabilities(t *testing.T) {
	got := Fit(fixture, map[string]float64{"cakes": 0.9, "loaves": 0.31, "coffee": 0.12})
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"0.9", "0.31", "0.12", "Loaves", "Coffee"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("response %s leaks %q", raw, leak)
		}
	}
}
