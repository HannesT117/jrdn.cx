package policy

import (
	"strings"
	"testing"

	"github.com/HannesT117/johannestrepesch.de/api/internal/content"
	"github.com/HannesT117/johannestrepesch.de/api/internal/model"
)

var bakery = content.Content{
	NoneIntent:          "nothing fits",
	GuardQuestion:       "instructions?",
	RelevanceQuestion:   "Does this fact help answer the question?",
	ComposeInstructions: "Write one or two sentences about Rosa's Bakery from the facts.",
	Topics: []content.Topic{
		{ID: "bread", Intent: "Bread and sourdough"},
		{ID: "hours", Intent: "Opening hours"},
	},
	Facts: []content.Fact{
		{ID: "sourdough", Topics: []string{"bread"}, Text: "The bakery bakes sourdough every morning."},
		{ID: "rye", Topics: []string{"bread"}, Text: "Rye bread is available on Saturdays."},
		{ID: "baguette", Topics: []string{"bread"}, Text: "Baguettes follow a French recipe."},
		{ID: "spelt", Topics: []string{"bread"}, Text: "Spelt loaves come out at nine."},
		{ID: "seeded", Topics: []string{"bread"}, Text: "Seeded loaves sell out by noon."},
		{ID: "weekdays", Topics: []string{"hours"}, Text: "The bakery opens at seven on weekdays."},
	},
	Strengths: []content.Criterion{{ID: "cakes", Label: "Cakes", Question: "cakes?"}},
}

func evaluation(topic string, p float64, scores map[string]float64) model.Evaluation {
	return model.Evaluation{Pick: model.Pick{ID: topic, P: p}, Scores: scores}
}

func TestAskQuestionsOfferTopicsPlusNoneAndEveryFact(t *testing.T) {
	choice, nouls := AskQuestions(bakery)
	if len(choice) != len(bakery.Topics)+1 || choice[len(choice)-1].ID != content.NoneID {
		t.Fatalf("got choice %+v", choice)
	}
	if len(nouls) != len(bakery.Facts) || !strings.Contains(nouls[0].Text, "sourdough every morning") {
		t.Fatalf("got nouls %+v", nouls)
	}
}

func TestSelectPicksTheMostRelevantFactsOfTheTopic(t *testing.T) {
	sel, ok := Select(bakery, evaluation("bread", 0.9, map[string]float64{
		"sourdough": 0.7, "rye": 0.95, "baguette": 0.2, "spelt": 0.8, "seeded": 0.6, "weekdays": 0.99,
	}))
	if !ok {
		t.Fatal("expected a selection")
	}
	if got := sel.Key(); got != "bread:rye,spelt,sourdough,seeded" {
		t.Fatalf("got key %q", got)
	}
}

func TestSelectFallsBackToTheBestFactWhenNoneIsClearlyRelevant(t *testing.T) {
	sel, ok := Select(bakery, evaluation("hours", 0.8, map[string]float64{"weekdays": 0.1}))
	if !ok || sel.Key() != "hours:weekdays" {
		t.Fatalf("got %q %v", sel.Key(), ok)
	}
}

func TestSelectRefusesUnclearOrUnknownTopics(t *testing.T) {
	for name, ev := range map[string]model.Evaluation{
		"none":            evaluation(content.NoneID, 0.99, nil),
		"unknown":         evaluation("cakes", 0.99, nil),
		"below threshold": evaluation("bread", AskThreshold-0.01, nil),
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := Select(bakery, ev); ok {
				t.Fatal("expected no selection")
			}
		})
	}
}

func TestParseKeyRoundTripsAndRejectsTampering(t *testing.T) {
	sel, ok := ParseKey(bakery, "bread:rye,sourdough")
	if !ok || sel.Key() != "bread:rye,sourdough" {
		t.Fatalf("got %q %v", sel.Key(), ok)
	}
	for _, key := range []string{
		"", "bread", "bread:", "cakes:rye", "bread:weekdays", "bread:rye,rye",
		"bread:rye,sourdough,baguette,spelt,seeded", "none:rye", "bread:invented",
	} {
		if _, ok := ParseKey(bakery, key); ok {
			t.Errorf("accepted %q", key)
		}
	}
}

func TestPromptContainsOnlyTrustedContent(t *testing.T) {
	sel, _ := ParseKey(bakery, "bread:rye,sourdough")
	system, user := Prompt(bakery, sel)
	if system != bakery.ComposeInstructions {
		t.Fatalf("got system %q", system)
	}
	for _, want := range []string{"Bread and sourdough", "Rye bread is available on Saturdays.", "The bakery bakes sourdough every morning."} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt misses %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "Spelt") {
		t.Error("user prompt contains an unselected fact")
	}
}

func TestVerifyKeepsFaithfulText(t *testing.T) {
	sel, _ := ParseKey(bakery, "bread:rye,sourdough")
	text := "Rosa's Bakery bakes sourdough every morning. Rye bread is there on Saturdays."
	if got := Verify(bakery, sel, text); got != text {
		t.Fatalf("got %q", got)
	}
}

func TestVerifyFallsBackToTheFactsForInventedDetails(t *testing.T) {
	sel, _ := ParseKey(bakery, "bread:rye,sourdough")
	fallback := "Rye bread is available on Saturdays. The bakery bakes sourdough every morning."
	for name, text := range map[string]string{
		"invented name":        "The bakery bakes sourdough with flour from Bavaria every morning.",
		"invented number":      "The bakery bakes 200 sourdough loaves every morning.",
		"invented number word": "Twenty bakers make sourdough every morning.",
		"invented opener":      "Google rates the sourdough highly.",
		"too long":             strings.Repeat("Sourdough every morning. ", 40),
		"empty":                "   ",
	} {
		t.Run(name, func(t *testing.T) {
			if got := Verify(bakery, sel, text); got != fallback {
				t.Fatalf("got %q", got)
			}
		})
	}
}
