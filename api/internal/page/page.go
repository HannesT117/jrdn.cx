package page

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HannesT117/johannestrepesch.de/api/internal/extras"
	"github.com/HannesT117/johannestrepesch.de/api/internal/garmin"
)

const (
	Days  = 4
	Slots = extras.MaxOpenBeans
	hues  = 12
)

var kinds = []string{"run", "ride", "other"}

var kindLabel = map[string]string{"run": "run", "ride": "ride", "other": "workout"}

//go:embed board.html
var boardHTML string

var board = template.Must(template.New("board").Funcs(template.FuncMap{
	"join": strings.Join,
	"num":  func(f float64) string { return strconv.FormatFloat(math.Round(f*100)/100, 'f', -1, 64) },
}).Parse(boardHTML))

var markers = []string{"coffee", "music", "sport", "reading"}

type Data struct {
	Extras        extras.Extras
	Activities    []garmin.Activity
	ActivityKnown bool
	Now           time.Time
	Zone          *time.Location
}

func Render(shell []byte, d Data) ([]byte, error) {
	views := map[string]any{
		"coffee":  coffee(d.Extras),
		"music":   music(d.Extras.Upcoming(d.Now.In(d.Zone))),
		"sport":   sport(d),
		"reading": reading(d.Extras),
	}
	out := shell
	for _, name := range markers {
		marker := []byte("<!--board:" + name + "-->")
		if !bytes.Contains(out, marker) {
			return nil, fmt.Errorf("page template has no %s", marker)
		}
		var buf bytes.Buffer
		if err := board.ExecuteTemplate(&buf, name, views[name]); err != nil {
			return nil, fmt.Errorf("render %s: %w", name, err)
		}
		out = bytes.Replace(out, marker, buf.Bytes(), 1)
	}
	return out, nil
}

type group struct {
	Kind  string
	Items []extras.Item
}

func plate(items []extras.Item) []group {
	var groups []group
	for _, it := range items {
		i := slices.IndexFunc(groups, func(g group) bool { return g.Kind == it.Kind })
		if i < 0 {
			groups = append(groups, group{Kind: it.Kind})
			i = len(groups) - 1
		}
		groups[i].Items = append(groups[i].Items, it)
	}
	return groups
}

type entry struct {
	Text, Note, Hue string
}

func hue(text string) string {
	h := 0
	for _, r := range text {
		h = (h*31 + int(r)) % 360
	}
	return "hue-" + strconv.Itoa(h*hues/360)
}

func logEntry(text, note string) entry {
	return entry{Text: text, Note: note, Hue: hue(text)}
}

func joinNonEmpty(parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), " · ")
}

func coffee(e extras.Extras) map[string]any {
	var slots []*extras.Bean
	if len(e.Beans) > 0 {
		slots = make([]*extras.Bean, Slots)
		for i := range e.Beans {
			slots[i] = &e.Beans[i]
		}
	}
	var past []entry
	for _, b := range e.PastBeans {
		past = append(past, logEntry(b.Name+" · "+b.Roaster, b.Origin))
	}
	return map[string]any{
		"Any":       len(e.Beans)+len(e.PastBeans)+len(e.Gear)+len(e.Brews) > 0,
		"Slots":     slots,
		"Brews":     e.Brews,
		"Gear":      plate(e.Gear),
		"PastBeans": past,
	}
}

type book struct {
	Title, Author, Hue string
}

func reading(e extras.Extras) map[string]any {
	var books []book
	for _, b := range e.Books {
		books = append(books, book{Title: b.Title, Author: b.Author, Hue: hue(b.Title)})
	}
	var past, podcasts []entry
	for _, b := range e.PastBooks {
		finished, _ := time.Parse(time.DateOnly, b.Finished)
		note := ""
		if !finished.IsZero() {
			note = finished.Format("Jan")
		}
		past = append(past, logEntry(b.Title+" · "+b.Author, note))
	}
	for _, p := range e.Podcasts {
		podcasts = append(podcasts, logEntry(joinNonEmpty(p.Name, p.Host), ""))
	}
	return map[string]any{
		"Any":       len(books)+len(past)+len(podcasts) > 0,
		"Books":     books,
		"PastBooks": past,
		"Podcasts":  podcasts,
	}
}

type gig struct {
	Date, Day, Month, Program, Where string
}

func music(e extras.Extras) map[string]any {
	var gigs []gig
	for _, g := range e.Gigs {
		date, err := time.Parse(time.DateOnly, g.Date)
		if err != nil {
			continue
		}
		gigs = append(gigs, gig{Date: g.Date, Day: strconv.Itoa(date.Day()), Month: date.Format("Jan"), Program: g.Program, Where: joinNonEmpty(g.Choir, g.Place)})
	}
	tally := "A few concerts:"
	if e.Concerts > 0 {
		tally = fmt.Sprintf("Around %d concerts. A few of them:", e.Concerts)
	}
	var repertoire []extras.Piece
	if len(e.Repertoire) > 0 {
		repertoire = e.Repertoire
	}
	return map[string]any{
		"Repertoire": repertoire,
		"Stages":     e.Stages,
		"Tally":      tally,
		"Countries":  slices.Sorted(slices.Values(e.Countries)),
		"Kit":        plate(e.Music),
		"Gigs":       gigs,
	}
}

type fill struct {
	Kind       string
	X, Y, W, H float64
}

type timeline struct {
	Level, State, Label string
	Count               int
	Hours, Now, Rest    float64
	Fills               []fill
	Ticks               []float64
	Days                []string
}

func sport(d Data) map[string]any {
	var tl *timeline
	if d.ActivityKnown {
		t := track(d.Activities, d.Now, d.Zone)
		tl = &t
	}
	bikes, devices := plate(d.Extras.Bikes), plate(d.Extras.SportGear)
	return map[string]any{
		"Any":      tl != nil || len(bikes)+len(devices) > 0,
		"Timeline": tl,
		"Bikes":    bikes,
		"Devices":  devices,
	}
}

func kindOf(kind string) string {
	if slices.Contains(kinds, kind) {
		return kind
	}
	return "other"
}

func track(acts []garmin.Activity, now time.Time, zone *time.Location) timeline {
	local := now.In(zone)
	midnight := func(offset int) time.Time {
		return time.Date(local.Year(), local.Month(), local.Day()+offset, 0, 0, 0, 0, zone)
	}
	start, end := midnight(1-Days), midnight(1)
	hours := func(t time.Time) float64 { return t.Sub(start).Hours() }
	t := timeline{Count: Days, Hours: hours(end), Now: hours(now), Rest: end.Sub(now).Hours()}

	var active []string
	for i := range Days {
		from, to := midnight(i+1-Days), midnight(i+2-Days)
		t.Days = append(t.Days, from.Weekday().String()[:3])
		if i > 0 {
			t.Ticks = append(t.Ticks, hours(from))
		}
		var present []string
		for _, k := range kinds {
			if slices.ContainsFunc(acts, func(a garmin.Activity) bool {
				return kindOf(a.Kind) == k && !a.Start.Before(from) && a.Start.Before(to)
			}) {
				present = append(present, k)
			}
		}
		height := 12 / float64(max(len(present), 1))
		var labels []string
		for j, k := range present {
			t.Fills = append(t.Fills, fill{Kind: k, X: hours(from), Y: float64(j) * height, W: to.Sub(from).Hours(), H: height})
			labels = append(labels, kindLabel[k])
		}
		if len(present) > 0 {
			active = append(active, t.Days[i]+" "+strings.Join(labels, " and "))
		}
	}
	if len(active) > 0 {
		t.Label = fmt.Sprintf("Active in the last %d days: %s", Days, strings.Join(active, ", "))
	} else {
		t.Label = fmt.Sprintf("No activity in the last %d days", Days)
	}

	var last time.Time
	for _, a := range acts {
		if e := a.Start.Add(time.Duration(a.Minutes) * time.Minute); e.After(last) {
			last = e
		}
	}
	if last.IsZero() {
		t.Level, t.State = "down", fmt.Sprintf("outage · nothing in %d days", Days)
		return t
	}
	ago := int(math.Round(max(0, now.Sub(last).Hours())))
	switch {
	case ago < 24:
		t.Level = "ok"
	case ago < 36:
		t.Level = "warn"
	default:
		t.Level = "down"
	}
	t.State = fmt.Sprintf("%s · last activity %dh ago", map[string]string{"ok": "operational", "warn": "degraded", "down": "outage"}[t.Level], ago)
	return t
}
