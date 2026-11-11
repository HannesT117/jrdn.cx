package extras

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var monday = time.Date(2031, 3, 3, 9, 0, 0, 0, time.UTC)

func writeExtras(t *testing.T, path string, e Extras) {
	t.Helper()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRoundTripsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extras.json")
	writeExtras(t, path, Extras{
		Beans:    []Bean{{Name: "Lumen", Origin: "Peru", Roaster: "R"}},
		Gear:     []Item{{Kind: "grinder", Name: "Flat 64"}},
		Brews:    []string{"V60"},
		Podcasts: []Podcast{{Name: "Crumb Talk", Host: "Ida Brenner"}},
	})
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Beans) != 1 || got.Beans[0].Name != "Lumen" || got.Gear[0].Name != "Flat 64" || got.Brews[0] != "V60" || got.Podcasts[0].Host != "Ida Brenner" {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadTreatsAMissingFileAsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(got.Beans) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestLoadRejectsInvalidFiles(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown field":     `{"beans":[],"secret":1}`,
		"five open beans":   `{"beans":[` + strings.Repeat(`{"name":"A","origin":"P","roaster":"R"},`, 4) + `{"name":"A","origin":"P","roaster":"R"}]}`,
		"bean with a date":  `{"beans":[{"name":"A","origin":"P","roaster":"R","opened":"2031-01-01"}]}`,
		"seven past books":  `{"pastBooks":[` + strings.TrimSuffix(strings.Repeat(`{"title":"A","author":"B","started":"2031-01-01"},`, 7), ",") + `]}`,
		"book bad date":     `{"books":[{"title":"A","author":"B","started":"yesterday"}]}`,
		"empty gear":        `{"gear":[{"kind":"grinder","name":""}]}`,
		"empty brew":        `{"brews":[""]}`,
		"podcast no name":   `{"podcasts":[{"name":"","host":"Ida Brenner"}]}`,
		"gig bad date":      `{"gigs":[{"date":"soon","program":"Mass","place":"Hall"}]}`,
		"piece no composer": `{"repertoire":[{"title":"Night Song","composer":"","year":1890,"form":"Motet"}]}`,
		"piece no form":     `{"repertoire":[{"title":"Night Song","composer":"Varga","year":1890,"form":""}]}`,
		"piece far future":  `{"repertoire":[{"title":"Night Song","composer":"Varga","year":2400,"form":"Motet"}]}`,
		"piece no year":     `{"repertoire":[{"title":"Night Song","composer":"Varga","form":"Motet"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "extras.json")
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLoadAlwaysReturnsListsForTheFrontend(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || got.PastBeans == nil || got.Books == nil || got.Gear == nil || got.Brews == nil || got.Bikes == nil || got.Podcasts == nil || got.Gigs == nil || got.Repertoire == nil {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestUpcomingHidesPastGigsWithoutTouchingTheRest(t *testing.T) {
	e := Extras{
		Beans: []Bean{{Name: "Lumen", Origin: "Peru", Roaster: "R"}},
		Gigs:  []Gig{{Date: "2031-03-02", Program: "Old", Place: "Hall"}, {Date: "2031-03-03", Program: "Today", Place: "Hall"}},
	}
	got := e.Upcoming(monday)
	if len(got.Gigs) != 1 || got.Gigs[0].Program != "Today" || len(got.Beans) != 1 || len(e.Gigs) != 2 {
		t.Fatalf("got %+v from %+v", got, e)
	}
}

func TestPodcastsNeedOnlyAName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extras.json")
	if err := os.WriteFile(path, []byte(`{"podcasts":[{"name":"Crumb Talk"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(path); err != nil || got.Podcasts[0].Name != "Crumb Talk" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestItemsCarryDetailsAndSportAndMusicKit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extras.json")
	raw := `{
		"bikes": [{"kind":"road","name":"Lark 7","details":["Carbon wheels","28 mm tyres"]}],
		"sportGear": [{"kind":"watch","name":"Pulse 3"}],
		"music": [{"kind":"voice","name":"Bass"}]
	}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Bikes[0].Details[1] != "28 mm tyres" || got.SportGear[0].Name != "Pulse 3" || got.Music[0].Name != "Bass" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestLoadRejectsBlankItemDetails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extras.json")
	if err := os.WriteFile(path, []byte(`{"bikes":[{"kind":"road","name":"Lark 7","details":[" "]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRepertoireKeepsEveryPieceField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extras.json")
	raw := `{"repertoire":[
		{"title":"Night Song","composer":"Varga","year":1890,"approx":true,"form":"Motet","scoring":"SATB","album":"Lanterns, 2019"},
		{"title":"River Mass","composer":"Okafor","year":2004,"form":"Mass"}
	]}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	want := Piece{Title: "Night Song", Composer: "Varga", Year: 1890, Approx: true, Form: "Motet", Scoring: "SATB", Album: "Lanterns, 2019"}
	if err != nil || len(got.Repertoire) != 2 || got.Repertoire[0] != want || got.Repertoire[1].Scoring != "" {
		t.Fatalf("got %+v, %v", got.Repertoire, err)
	}
}

func TestRepertoireFitsHundredsOfPieces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extras.json")
	piece := `{"title":"A rather long title for a motet in five parts","composer":"Varga","year":1890,"form":"Motet","scoring":"SATB divisi","album":"Lanterns in the Old Harbour Hall, 2019"}`
	raw := `{"repertoire":[` + strings.TrimSuffix(strings.Repeat(piece+",", MaxRepertoire), ",") + `]}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(path); err != nil || len(got.Repertoire) != MaxRepertoire {
		t.Fatalf("got %d pieces, %v", len(got.Repertoire), err)
	}
	more := `{"repertoire":[` + strings.Repeat(piece+",", MaxRepertoire) + piece + `]}`
	if err := os.WriteFile(path, []byte(more), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error above the limit")
	}
}

func TestStagesKeepHighlightsPlacesAndACount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extras.json")
	raw := `{
		"concerts": 120,
		"stages": [
			{"year":2019,"title":"Evening of motets","place":"Harbour Hall","with":"Lantern Singers"},
			{"title":"Singing on a bike tour"}
		],
		"countries": ["Tallinn", "Porto"]
	}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	want := Stage{Year: 2019, Title: "Evening of motets", Place: "Harbour Hall", With: "Lantern Singers"}
	if err != nil || got.Concerts != 120 || len(got.Stages) != 2 || got.Stages[0] != want || got.Stages[1].Year != 0 || got.Countries[1] != "Porto" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if empty, _ := Load(filepath.Join(t.TempDir(), "missing.json")); empty.Stages == nil || empty.Countries == nil {
		t.Fatalf("lists must not be nil: %#v", empty)
	}
}

func TestLoadRejectsInvalidStages(t *testing.T) {
	for name, raw := range map[string]string{
		"no title":       `{"stages":[{"year":2019,"place":"Harbour Hall"}]}`,
		"year too early": `{"stages":[{"year":200,"title":"Motets"}]}`,
		"blank country":  `{"countries":[" "]}`,
		"negative count": `{"concerts":-1}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "extras.json")
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
