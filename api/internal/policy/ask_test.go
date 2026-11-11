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
	GroundingQuestion:   "Does the answer stay within the facts?",
	Topics: []content.Topic{
		{ID: "bread", Intent: "Bread and sourdough"},
		{ID: "hours", Intent: "Opening hours"},
		{ID: "coffee", Intent: "Coffee"},
		{ID: "pastry", Intent: "Pastries"},
	},
	Facts: []content.Fact{
		{ID: "sourdough", Topics: []string{"bread"}, Text: "The bakery bakes sourdough every morning."},
		{ID: "rye", Topics: []string{"bread"}, Text: "Rye bread is available on Saturdays."},
		{ID: "baguette", Topics: []string{"bread"}, Text: "Baguettes follow a French recipe."},
		{ID: "spelt", Topics: []string{"bread"}, Text: "Spelt loaves come out at nine."},
		{ID: "seeded", Topics: []string{"bread"}, Text: "Seeded loaves sell out by noon."},
		{ID: "weekdays", Topics: []string{"hours"}, Text: "The bakery opens at seven on weekdays."},
		{ID: "espresso", Topics: []string{"coffee"}, Text: "Espresso comes from a lever machine."},
		{ID: "croissant", Topics: []string{"pastry"}, Text: "Croissants are baked with French butter."},
	},
	Strengths: []content.Criterion{{ID: "cakes", Label: "Cakes", Question: "cakes?"}},
}

func evaluation(topics, scores map[string]float64) model.Evaluation {
	return model.Evaluation{Topics: topics, Scores: scores}
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
	sel, ok := Select(bakery, evaluation(map[string]float64{"bread": 0.9, content.NoneID: 0.01}, map[string]float64{
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
	sel, ok := Select(bakery, evaluation(map[string]float64{"hours": 0.8}, map[string]float64{"weekdays": 0.1}))
	if !ok || sel.Key() != "hours:weekdays" {
		t.Fatalf("got %q %v", sel.Key(), ok)
	}
}

func TestSelectCombinesTopicsUntilTheyCoverTheQuestion(t *testing.T) {
	sel, ok := Select(bakery, evaluation(
		map[string]float64{"bread": 0.45, "hours": 0.3, "coffee": 0.15, content.NoneID: 0.02},
		map[string]float64{"weekdays": 0.9, "rye": 0.8, "sourdough": 0.3, "espresso": 0.99},
	))
	if !ok || sel.Key() != "bread+hours:weekdays,rye" {
		t.Fatalf("got %q %v", sel.Key(), ok)
	}
}

func TestSelectTakesAtMostMaxTopics(t *testing.T) {
	sel, ok := Select(bakery, evaluation(
		map[string]float64{"bread": 0.21, "hours": 0.2, "coffee": 0.19, "pastry": 0.18, content.NoneID: 0.02},
		map[string]float64{"rye": 0.9, "weekdays": 0.9, "espresso": 0.9, "croissant": 0.9},
	))
	if !ok || len(sel.Topics) != MaxTopics || strings.Contains(sel.Key(), "croissant") {
		t.Fatalf("got %q %v", sel.Key(), ok)
	}
}

func TestSelectRefusesOffTopicDiffuseOrUnknownQuestions(t *testing.T) {
	for name, topics := range map[string]map[string]float64{
		"none wins":       {content.NoneID: 0.9, "bread": 0.05},
		"none too likely": {"bread": 0.55, content.NoneID: NoneCeiling},
		"diffuse":         {"bread": 0.15, "hours": 0.15, "coffee": 0.15, "pastry": 0.15, content.NoneID: 0.1},
		"unknown topic":   {"cakes": 0.99},
		"no distribution": nil,
	} {
		t.Run(name, func(t *testing.T) {
			if sel, ok := Select(bakery, evaluation(topics, map[string]float64{"rye": 0.9})); ok {
				t.Fatalf("expected no selection, got %q", sel.Key())
			}
		})
	}
}

func selection(topic string, factIDs ...string) Selection {
	sel := Selection{}
	for _, t := range bakery.Topics {
		if t.ID == topic {
			sel.Topics = append(sel.Topics, t)
		}
	}
	for _, id := range factIDs {
		for _, f := range bakery.Facts {
			if f.ID == id {
				sel.Facts = append(sel.Facts, f)
			}
		}
	}
	return sel
}

func TestPromptQuotesTheQuestionNextToTheSelectedFacts(t *testing.T) {
	system, user := Prompt(bakery, selection("bread", "rye", "sourdough"), `When is rye "fresh"?`)
	if system != bakery.ComposeInstructions {
		t.Fatalf("got system %q", system)
	}
	for _, want := range []string{`Question: "When is rye \"fresh\"?"`, "Bread and sourdough", "Rye bread is available on Saturdays.", "The bakery bakes sourdough every morning."} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt misses %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "Spelt") {
		t.Error("user prompt contains an unselected fact")
	}
}

func TestAnswerIDDependsOnSelectionAndQuestionOnly(t *testing.T) {
	sel := selection("bread", "rye")
	id := AnswerID(sel, "When is rye bread available?")
	if id == "" || strings.ContainsAny(id, ":/ ") {
		t.Fatalf("got id %q", id)
	}
	if got := AnswerID(sel, "  when IS rye   bread available?"); got != id {
		t.Errorf("same question in other spelling got %q, want %q", got, id)
	}
	if AnswerID(sel, "Is rye bread sold on Sundays?") == id || AnswerID(selection("bread", "rye", "sourdough"), "When is rye bread available?") == id {
		t.Error("different question or facts share an id")
	}
}

func TestGroundingCheckAsksWhetherTheAnswerStaysWithinTheFacts(t *testing.T) {
	state, nouls := GroundingCheck(bakery, selection("bread", "rye"), "Rye bread is there on Saturdays.")
	if !strings.Contains(state, "Rye bread is available on Saturdays.") || !strings.Contains(state, "Rye bread is there on Saturdays.") || strings.Contains(state, "sourdough every morning") {
		t.Fatalf("got state %q", state)
	}
	if len(nouls) != 1 || nouls[0].Text != bakery.GroundingQuestion {
		t.Fatalf("got nouls %+v", nouls)
	}
	if !Grounded(map[string]float64{nouls[0].ID: GroundingThreshold}) || Grounded(map[string]float64{nouls[0].ID: GroundingThreshold - 0.01}) || Grounded(nil) {
		t.Fatal("threshold not applied")
	}
}

func TestVerifyKeepsFaithfulText(t *testing.T) {
	sel := selection("bread", "rye", "sourdough")
	text := "Rosa's Bakery bakes sourdough every morning. Rye bread is there on Saturdays."
	if got := Verify(bakery, sel, text); got != text {
		t.Fatalf("got %q", got)
	}
}

func TestVerifyFallsBackToTheFactsForInventedDetails(t *testing.T) {
	sel := selection("bread", "rye", "sourdough")
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
