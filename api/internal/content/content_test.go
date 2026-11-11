package content

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleContentIsValid(t *testing.T) {
	c, err := Load("testdata/content.example.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Topics) == 0 || len(c.Facts) == 0 || len(c.Strengths) == 0 {
		t.Fatalf("expected topics, facts and strengths, got %d, %d and %d", len(c.Topics), len(c.Facts), len(c.Strengths))
	}
}

func TestLoadFailsForMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFactsForTopicKeepsContentOrder(t *testing.T) {
	c := Content{Facts: []Fact{
		{ID: "a", Topics: []string{"bread"}, Text: "A."},
		{ID: "b", Topics: []string{"hours"}, Text: "B."},
		{ID: "c", Topics: []string{"hours", "bread"}, Text: "C."},
	}}
	got := c.FactsFor("bread")
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseRejectsInvalidContent(t *testing.T) {
	const texts = `"relevanceQuestion":"r","composeInstructions":"c",`
	const head = `"noneIntent":"n","guardQuestion":"g",` + texts
	const strengths = `"strengths":[{"id":"s","label":"l","question":"q"}]`
	const topics = `"topics":[{"id":"bread","intent":"bread"}]`
	const facts = `"facts":[{"id":"f1","topics":["bread"],"text":"Sourdough."}]`
	cases := map[string]string{
		"no relevance":       `{"noneIntent":"n","guardQuestion":"g","composeInstructions":"c",` + topics + `,` + facts + `,` + strengths + `}`,
		"no compose":         `{"noneIntent":"n","guardQuestion":"g","relevanceQuestion":"r",` + topics + `,` + facts + `,` + strengths + `}`,
		"no none intent":     `{"guardQuestion":"g",` + texts + topics + `,` + facts + `,` + strengths + `}`,
		"no guard":           `{"noneIntent":"n",` + texts + topics + `,` + facts + `,` + strengths + `}`,
		"no topics":          `{` + head + `"topics":[],` + facts + `,` + strengths + `}`,
		"no facts":           `{` + head + topics + `,"facts":[],` + strengths + `}`,
		"no strengths":       `{` + head + topics + `,` + facts + `,"strengths":[]}`,
		"reserved topic":     `{` + head + `"topics":[{"id":"none","intent":"x"}],"facts":[{"id":"f1","topics":["none"],"text":"t"}],` + strengths + `}`,
		"duplicate topic":    `{` + head + `"topics":[{"id":"bread","intent":"x"},{"id":"bread","intent":"y"}],` + facts + `,` + strengths + `}`,
		"empty intent":       `{` + head + `"topics":[{"id":"bread","intent":""}],` + facts + `,` + strengths + `}`,
		"duplicate fact":     `{` + head + topics + `,"facts":[{"id":"f1","topics":["bread"],"text":"a"},{"id":"f1","topics":["bread"],"text":"b"}],` + strengths + `}`,
		"empty fact text":    `{` + head + topics + `,"facts":[{"id":"f1","topics":["bread"],"text":""}],` + strengths + `}`,
		"fact without topic": `{` + head + topics + `,"facts":[{"id":"f1","topics":["bread"],"text":"a"},{"id":"f2","topics":[],"text":"b"}],` + strengths + `}`,
		"unknown fact topic": `{` + head + topics + `,"facts":[{"id":"f1","topics":["cake"],"text":"a"}],` + strengths + `}`,
		"topic without fact": `{` + head + `"topics":[{"id":"bread","intent":"x"},{"id":"hours","intent":"y"}],` + facts + `,` + strengths + `}`,
		"empty label":        `{` + head + topics + `,` + facts + `,"strengths":[{"id":"s","label":"","question":"q"}]}`,
		"criterion clash":    `{` + head + topics + `,` + facts + `,` + strengths + `,"learning":[{"id":"s","label":"l","question":"q"}]}`,
		"guard id taken":     `{` + head + topics + `,` + facts + `,"strengths":[{"id":"guard","label":"l","question":"q"}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(raw)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
