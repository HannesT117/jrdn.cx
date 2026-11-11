package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

const (
	NoneID  = "none"
	GuardID = "guard"
)

type Topic struct {
	ID     string `json:"id"`
	Intent string `json:"intent"`
}

type Fact struct {
	ID     string   `json:"id"`
	Topics []string `json:"topics"`
	Text   string   `json:"text"`
}

type Criterion struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Question string `json:"question"`
}

type Content struct {
	NoneIntent          string      `json:"noneIntent"`
	GuardQuestion       string      `json:"guardQuestion"`
	RelevanceQuestion   string      `json:"relevanceQuestion"`
	ComposeInstructions string      `json:"composeInstructions"`
	GroundingQuestion   string      `json:"groundingQuestion"`
	Topics              []Topic     `json:"topics"`
	Facts               []Fact      `json:"facts"`
	Strengths           []Criterion `json:"strengths"`
	Learning            []Criterion `json:"learning"`
}

func Load(path string) (Content, error) {
	f, err := os.Open(path)
	if err != nil {
		return Content{}, err
	}
	defer f.Close()
	return Parse(f)
}

func Parse(r io.Reader) (Content, error) {
	var c Content
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Content{}, fmt.Errorf("decode content: %w", err)
	}
	if err := c.validate(); err != nil {
		return Content{}, err
	}
	return c, nil
}

func (c Content) FactsFor(topic string) []Fact {
	var facts []Fact
	for _, f := range c.Facts {
		if slices.Contains(f.Topics, topic) {
			facts = append(facts, f)
		}
	}
	return facts
}

func (c Content) validate() error {
	if c.NoneIntent == "" || c.GuardQuestion == "" || c.RelevanceQuestion == "" || c.ComposeInstructions == "" || c.GroundingQuestion == "" || len(c.Topics) == 0 || len(c.Facts) == 0 || len(c.Strengths) == 0 {
		return errors.New("content needs a none intent, guard, relevance, compose and grounding texts, topics, facts and strengths")
	}
	topicIDs := map[string]bool{NoneID: true}
	for _, t := range c.Topics {
		if err := claim(topicIDs, t.ID); err != nil {
			return err
		}
		if t.Intent == "" {
			return fmt.Errorf("topic %q needs an intent", t.ID)
		}
	}
	factIDs := map[string]bool{}
	for _, f := range c.Facts {
		if err := claim(factIDs, f.ID); err != nil {
			return err
		}
		if f.Text == "" || len(f.Topics) == 0 {
			return fmt.Errorf("fact %q needs text and at least one topic", f.ID)
		}
		for _, t := range f.Topics {
			if t == NoneID || !topicIDs[t] {
				return fmt.Errorf("fact %q has unknown topic %q", f.ID, t)
			}
		}
	}
	for _, t := range c.Topics {
		if len(c.FactsFor(t.ID)) == 0 {
			return fmt.Errorf("topic %q has no facts", t.ID)
		}
	}
	criterionIDs := map[string]bool{GuardID: true}
	for _, list := range [][]Criterion{c.Strengths, c.Learning} {
		for _, cr := range list {
			if err := claim(criterionIDs, cr.ID); err != nil {
				return err
			}
			if cr.Label == "" || cr.Question == "" {
				return fmt.Errorf("criterion %q needs label and question", cr.ID)
			}
		}
	}
	return nil
}

func claim(seen map[string]bool, id string) error {
	if id == "" || seen[id] {
		return fmt.Errorf("content id %q is empty, reserved or duplicated", id)
	}
	seen[id] = true
	return nil
}
