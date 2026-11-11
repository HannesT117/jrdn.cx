package policy

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/HannesT117/johannestrepesch.de/api/internal/content"
	"github.com/HannesT117/johannestrepesch.de/api/internal/model"
)

const (
	NoneCeiling   = 0.2
	CoverTopics   = 0.7
	AskThreshold  = 0.5
	MaxTopics     = 3
	FactThreshold = 0.5
	MaxFacts      = 5
	maxAnswerLen  = 600

	GroundingThreshold = 0.7
	groundedID         = "grounded"
)

type Selection struct {
	Topics []content.Topic
	Facts  []content.Fact
}

func (s Selection) Key() string {
	topics := make([]string, len(s.Topics))
	for i, t := range s.Topics {
		topics[i] = t.ID
	}
	facts := make([]string, len(s.Facts))
	for i, f := range s.Facts {
		facts[i] = f.ID
	}
	return strings.Join(topics, "+") + ":" + strings.Join(facts, ",")
}

func Answer(key, text string) Response {
	return Response{Kind: KindAnswer, ID: key, Text: text}
}

func AskQuestions(c content.Content) (choice, nouls []model.Option) {
	for _, t := range c.Topics {
		choice = append(choice, model.Option{ID: t.ID, Text: t.Intent})
	}
	choice = append(choice, model.Option{ID: content.NoneID, Text: c.NoneIntent})
	for _, f := range c.Facts {
		nouls = append(nouls, model.Option{ID: f.ID, Text: c.RelevanceQuestion + " Fact: " + f.Text})
	}
	return choice, nouls
}

func Select(c content.Content, ev model.Evaluation) (Selection, bool) {
	if ev.Topics[content.NoneID] >= NoneCeiling {
		return Selection{}, false
	}
	ranked := slices.Clone(c.Topics)
	slices.SortStableFunc(ranked, func(a, b content.Topic) int { return cmp.Compare(ev.Topics[b.ID], ev.Topics[a.ID]) })
	var sel Selection
	covered := 0.0
	for _, t := range ranked[:min(MaxTopics, len(ranked))] {
		if covered >= CoverTopics || ev.Topics[t.ID] == 0 {
			break
		}
		sel.Topics = append(sel.Topics, t)
		covered += ev.Topics[t.ID]
	}
	if covered < AskThreshold {
		return Selection{}, false
	}

	var candidates []content.Fact
	for _, f := range c.Facts {
		if slices.ContainsFunc(sel.Topics, func(t content.Topic) bool { return slices.Contains(f.Topics, t.ID) }) {
			candidates = append(candidates, f)
		}
	}
	slices.SortStableFunc(candidates, func(a, b content.Fact) int { return cmp.Compare(ev.Scores[b.ID], ev.Scores[a.ID]) })
	for _, f := range candidates {
		if len(sel.Facts) < MaxFacts && ev.Scores[f.ID] >= FactThreshold {
			sel.Facts = append(sel.Facts, f)
		}
	}
	if len(sel.Facts) == 0 {
		sel.Facts = candidates[:1]
	}
	return sel, true
}

func AnswerID(sel Selection, question string) string {
	sum := sha256.Sum256([]byte(sel.Key() + "\n" + strings.Join(strings.Fields(strings.ToLower(question)), " ")))
	return hex.EncodeToString(sum[:16])
}

func Prompt(c content.Content, sel Selection, question string) (system, user string) {
	return c.ComposeInstructions, "Question: " + strconv.Quote(question) + "\n" + factSheet(sel)
}

func GroundingCheck(c content.Content, sel Selection, answer string) (state string, nouls []model.Option) {
	return factSheet(sel) + "Answer: " + answer, []model.Option{{ID: groundedID, Text: c.GroundingQuestion}}
}

func Grounded(scores map[string]float64) bool {
	return scores[groundedID] >= GroundingThreshold
}

func factSheet(sel Selection) string {
	var b strings.Builder
	b.WriteString("Topics:\n")
	for _, t := range sel.Topics {
		b.WriteString("- " + t.Intent + "\n")
	}
	b.WriteString("Facts:\n")
	for _, f := range sel.Facts {
		b.WriteString("- " + f.Text + "\n")
	}
	return b.String()
}

var (
	wordPattern     = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}'’+#.-]*[\p{L}\p{N}]|[\p{L}\p{N}]`)
	sentenceBreak   = regexp.MustCompile(`[.!?]\s+`)
	numberWords     = set("one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "fifteen", "twenty", "thirty", "fifty", "hundred", "hundreds", "thousand", "thousands", "million", "millions", "dozen", "dozens")
	sentenceOpeners = set("He", "His", "The", "At", "In", "On", "As", "With", "For", "This", "That", "These", "Before", "After", "And", "But", "Also", "Both", "There", "It", "Over", "During", "While", "Since", "From", "To", "When", "Through", "Besides", "Today", "Currently", "Previously", "Earlier", "Later", "A", "An", "Most", "Each", "Every", "Beyond", "Alongside", "Outside", "Across")
)

func Verify(c content.Content, sel Selection, text string) string {
	fallback := make([]string, len(sel.Facts))
	for i, f := range sel.Facts {
		fallback[i] = f.Text
	}
	text = strings.TrimSpace(text)
	if text == "" || len(text) > maxAnswerLen || !faithful(trusted(c, sel), text) {
		return strings.Join(fallback, " ")
	}
	return text
}

func trusted(c content.Content, sel Selection) map[string]bool {
	words := map[string]bool{}
	sources := []string{c.ComposeInstructions}
	for _, t := range sel.Topics {
		sources = append(sources, t.Intent)
	}
	for _, f := range sel.Facts {
		sources = append(sources, f.Text)
	}
	for _, s := range sources {
		for _, w := range wordPattern.FindAllString(s, -1) {
			words[w] = true
			words[strings.ToLower(w)] = true
		}
	}
	return words
}

func faithful(allowed map[string]bool, text string) bool {
	for _, sentence := range sentenceBreak.Split(text, -1) {
		for i, w := range wordPattern.FindAllString(sentence, -1) {
			w = strings.TrimRight(w, ".")
			lower := strings.ToLower(w)
			risky := strings.ContainsAny(w, "0123456789") || numberWords[lower] || (w != lower && !(i == 0 && sentenceOpeners[w]))
			if risky && !allowed[w] && !(i == 0 && allowed[lower]) {
				return false
			}
		}
	}
	return true
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
