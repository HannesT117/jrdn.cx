package page

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/HannesT117/johannestrepesch.de/api/internal/extras"
	"github.com/HannesT117/johannestrepesch.de/api/internal/garmin"
)

var sydney = mustZone("Australia/Sydney")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

const shell = `<!doctype html><title>t</title><main>
<!--board:coffee-->
<section class="node n-music"><!--board:music--></section>
<!--board:sport-->
<!--board:reading-->
</main>`

var now = time.Date(2031, 3, 5, 14, 0, 0, 0, sydney)

func render(t *testing.T, d Data) string {
	t.Helper()
	if d.Now.IsZero() {
		d.Now = now
	}
	if d.Zone == nil {
		d.Zone = sydney
	}
	out, err := Render([]byte(shell), d)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return string(out)
}

var spaces = regexp.MustCompile(`\s+`)

func has(t *testing.T, out string, parts ...string) {
	t.Helper()
	flat := spaces.ReplaceAllString(out, " ")
	for _, p := range parts {
		if !strings.Contains(flat, p) {
			t.Errorf("missing %q in\n%s", p, flat)
		}
	}
}

func lacks(t *testing.T, out string, parts ...string) {
	t.Helper()
	flat := spaces.ReplaceAllString(out, " ")
	for _, p := range parts {
		if strings.Contains(flat, p) {
			t.Errorf("unexpected %q in\n%s", p, flat)
		}
	}
}

func TestRenderFillsEveryMarkerAndKeepsTheShell(t *testing.T) {
	out := render(t, Data{})
	has(t, out, "<!doctype html><title>t</title>", `<section class="node n-music">`)
	lacks(t, out, "<!--board:")
}

func TestRenderRefusesATemplateWithoutMarkers(t *testing.T) {
	if _, err := Render([]byte("<main></main>"), Data{Now: now, Zone: sydney}); err == nil || !strings.Contains(err.Error(), "board:coffee") {
		t.Fatalf("got %v", err)
	}
}

func TestEmptyLanesStayHidden(t *testing.T) {
	out := render(t, Data{})
	has(t, out, `<div class="lane" id="feed-coffee" hidden>`, `<div class="lane" id="feed-sport" hidden>`, `<div class="lane" id="feed-reading" hidden>`)
}

func TestCoffeeShowsFourSlotsGroupedGearAndHistory(t *testing.T) {
	out := render(t, Data{Extras: extras.Extras{
		Beans:     []extras.Bean{{Name: "Lumen", Origin: "Peru", Process: "washed", Roaster: "Kiln & Co"}},
		PastBeans: []extras.Bean{{Name: "Dusk", Origin: "Kenya", Roaster: "Kiln & Co"}},
		Gear:      []extras.Item{{Kind: "grinder", Name: "Flat 64"}, {Kind: "scale", Name: "Pocket"}, {Kind: "grinder", Name: "Hand 1"}},
		Brews:     []string{"V60"},
	}})
	has(t, out,
		`<div class="lane" id="feed-coffee">`,
		`<li class="hopper"><b>Lumen</b> <small>Peru · washed</small> <small>Kiln &amp; Co</small></li>`,
		`<small>free slot</small>`,
		`<dt>grinder</dt> <dd>Flat 64</dd> <dd>Hand 1</dd> <dt>scale</dt> <dd>Pocket</dd>`,
		`<li>V60</li>`,
		`<span>Dusk · Kiln &amp; Co</span> <small>Kenya</small>`,
	)
	if got := strings.Count(out, `class="hopper empty"`); got != 3 {
		t.Errorf("%d free slots, want 3", got)
	}
	if !regexp.MustCompile(`<li class="hue-\d+"><span>Dusk`).MatchString(out) {
		t.Errorf("history entry without a hue class")
	}
}

func TestReadingShowsBooksHistoryAndPodcasts(t *testing.T) {
	out := render(t, Data{Extras: extras.Extras{
		Books:     []extras.Book{{Title: "The Salt Road", Author: "Ida Brenner", Started: "2031-01-01"}},
		PastBooks: []extras.Book{{Title: "Glass Harbour", Author: "Ola Wren", Started: "2030-11-01", Finished: "2030-12-20"}},
		Podcasts:  []extras.Podcast{{Name: "Crumb Talk", Host: "Ida Brenner"}, {Name: "Night Shift"}},
	}})
	has(t, out,
		`<div class="lane" id="feed-reading">`,
		`<cite>The Salt Road</cite> <span>Ida Brenner</span>`,
		`<span>Glass Harbour · Ola Wren</span> <small>Dec</small>`,
		`<span>Crumb Talk · Ida Brenner</span>`,
		`<span>Night Shift</span>`,
	)
}

func TestMusicShowsStagesSortedCountriesKitAndGigs(t *testing.T) {
	out := render(t, Data{Extras: extras.Extras{
		Concerts:  120,
		Stages:    []extras.Stage{{Year: 2012, Title: "Evening of motets", Place: "Harbour Hall", With: "Lantern Singers"}, {Title: "Bike tour"}},
		Countries: []string{"Portugal", "Estonia"},
		Music:     []extras.Item{{Kind: "voice", Name: "Bass"}},
		Gigs:      []extras.Gig{{Date: "2031-05-01", Program: "Spring Mass", Choir: "Rosa Singers", Place: "St Ann"}},
	}})
	has(t, out,
		`<p class="tally">Around 120 concerts. A few of them:</p>`,
		`<span><b>Evening of motets</b>, Harbour Hall</span> <small>2012</small> <small class="with">Lantern Singers</small>`,
		`<span><b>Bike tour</b></span> <small></small>`,
		`<li>Estonia</li> <li>Portugal</li>`,
		`<dt>voice</dt> <dd>Bass</dd>`,
		`<time datetime="2031-05-01"><b>1</b>May</time>`,
		`<cite>Spring Mass</cite> <span>Rosa Singers · St Ann</span>`,
	)
}

func TestMusicHidesGigsThatHavePassed(t *testing.T) {
	out := render(t, Data{Extras: extras.Extras{Gigs: []extras.Gig{{Date: "2031-03-04", Program: "Old Mass", Place: "St Ann"}}}})
	lacks(t, out, "Old Mass")
}

func TestRepertoireTravelsAsJSONForTheCloud(t *testing.T) {
	out := render(t, Data{Extras: extras.Extras{Repertoire: []extras.Piece{{Title: `Night "Song" & Day`, Composer: "Varga", Year: 1890, Form: "Motet"}}}})
	has(t, out, `<div class="cloud"`, `<script type="application/json" id="repertoire-data">`)
	lacks(t, out, `"Song" & Day`)
	if !strings.Contains(out, `"composer":"Varga"`) {
		t.Errorf("repertoire JSON missing composer")
	}
}

func TestSportTimelineCoversFourSydneyDays(t *testing.T) {
	acts := []garmin.Activity{
		{Kind: "run", Start: time.Date(2031, 3, 5, 6, 0, 0, 0, sydney).UTC(), Minutes: 45},
		{Kind: "ride", Start: time.Date(2031, 3, 5, 11, 0, 0, 0, sydney).UTC(), Minutes: 60},
		{Kind: "swim", Start: time.Date(2031, 3, 3, 7, 0, 0, 0, sydney).UTC(), Minutes: 30},
	}
	out := render(t, Data{Activities: acts, ActivityKnown: true, Extras: extras.Extras{Bikes: []extras.Item{{Kind: "road", Name: "Lark 7", Details: []string{"Carbon wheels", "28 mm"}}}}})
	has(t, out,
		`<div class="lane" id="feed-sport">`,
		`<p class="state s-ok">operational · last activity 2h ago</p>`,
		`<h3 class="sub">Last 4 days on Garmin</h3>`,
		`viewBox="0 0 96 12"`,
		`<rect class="t-other" x="24" y="0" width="24" height="12"></rect>`,
		`<rect class="t-run" x="72" y="0" width="24" height="6"></rect>`,
		`<rect class="t-ride" x="72" y="6" width="24" height="6"></rect>`,
		`<rect class="later" x="86" y="0" width="10" height="12"></rect>`,
		`aria-label="Active in the last 4 days: Mon workout, Wed run and ride"`,
		`<li>Sun</li> <li>Mon</li> <li>Tue</li> <li>Wed</li>`,
		`<dd>Lark 7<small>Carbon wheels · 28 mm</small></dd>`,
	)
}

func TestSportStateWhenNothingHappened(t *testing.T) {
	out := render(t, Data{ActivityKnown: true})
	has(t, out, `<p class="state s-down">outage · nothing in 4 days</p>`, `aria-label="No activity in the last 4 days"`)
}

func TestSportHidesTheTimelineWhenGarminIsUnknown(t *testing.T) {
	out := render(t, Data{Extras: extras.Extras{Bikes: []extras.Item{{Kind: "road", Name: "Lark 7"}}}})
	has(t, out, `<div class="lane" id="feed-sport">`)
	lacks(t, out, `class="track"`, `class="state`)
}

func TestTimelineHandlesTheDaylightSavingSwitch(t *testing.T) {
	afterSwitch := time.Date(2031, 4, 8, 12, 0, 0, 0, sydney)
	out := render(t, Data{Now: afterSwitch, ActivityKnown: true})
	has(t, out, `viewBox="0 0 97 12"`)
}
