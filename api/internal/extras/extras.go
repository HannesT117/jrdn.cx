package extras

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxOpenBeans  = 4
	MaxReading    = 3
	MaxHistory    = 6
	MaxGigs       = 8
	MaxRunes      = 80
	MaxRepertoire = 400
	firstYear     = 1000
	lastYear      = 2100
	dateLayout    = time.DateOnly
)

var ErrStale = errors.New("extras file is invalid, serving the last good version")

type Bean struct {
	Name    string `json:"name"`
	Origin  string `json:"origin"`
	Process string `json:"process,omitempty"`
	Roaster string `json:"roaster"`
}

type Book struct {
	Title    string `json:"title"`
	Author   string `json:"author"`
	Started  string `json:"started"`
	Finished string `json:"finished,omitempty"`
}

type Item struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Details []string `json:"details,omitempty"`
}

type Podcast struct {
	Name string `json:"name"`
	Host string `json:"host,omitempty"`
}

type Gig struct {
	Date    string `json:"date"`
	Program string `json:"program"`
	Choir   string `json:"choir,omitempty"`
	Place   string `json:"place"`
}

type Piece struct {
	Title    string `json:"title"`
	Composer string `json:"composer"`
	Year     int    `json:"year"`
	Approx   bool   `json:"approx,omitempty"`
	Form     string `json:"form"`
	Scoring  string `json:"scoring,omitempty"`
	Album    string `json:"album,omitempty"`
}

type Stage struct {
	Year  int    `json:"year,omitempty"`
	Title string `json:"title"`
	Place string `json:"place,omitempty"`
	With  string `json:"with,omitempty"`
}

type Extras struct {
	Beans      []Bean    `json:"beans"`
	PastBeans  []Bean    `json:"pastBeans"`
	Gear       []Item    `json:"gear"`
	Brews      []string  `json:"brews"`
	Bikes      []Item    `json:"bikes"`
	SportGear  []Item    `json:"sportGear"`
	Music      []Item    `json:"music"`
	Books      []Book    `json:"books"`
	PastBooks  []Book    `json:"pastBooks"`
	Podcasts   []Podcast `json:"podcasts"`
	Gigs       []Gig     `json:"gigs"`
	Repertoire []Piece   `json:"repertoire"`
	Concerts   int       `json:"concerts,omitempty"`
	Stages     []Stage   `json:"stages"`
	Countries  []string  `json:"countries"`
}

func (e Extras) Upcoming(now time.Time) Extras {
	today := now.Format(dateLayout)
	e = e.clone()
	e.Gigs = slices.DeleteFunc(e.Gigs, func(g Gig) bool { return g.Date < today })
	return e
}

func Load(path string) (Extras, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Extras{}.clone(), nil
	}
	if err != nil {
		return Extras{}, err
	}
	defer f.Close()
	var e Extras
	if err := decodeStrict(f, &e); err != nil {
		return Extras{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := e.validate(); err != nil {
		return Extras{}, fmt.Errorf("%s: %w", path, err)
	}
	return e.clone(), nil
}

type Store struct {
	path string

	mu      sync.Mutex
	modTime time.Time
	size    int64
	loaded  bool
	current Extras
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Get() (Extras, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.loaded, s.current, s.modTime, s.size = true, Extras{}.clone(), time.Time{}, 0
		return s.current, nil
	}
	if err != nil {
		return s.current, errors.Join(ErrStale, err)
	}
	if s.loaded && info.ModTime().Equal(s.modTime) && info.Size() == s.size {
		return s.current, nil
	}
	e, err := Load(s.path)
	if err != nil {
		return s.current, errors.Join(ErrStale, err)
	}
	s.loaded, s.current, s.modTime, s.size = true, e, info.ModTime(), info.Size()
	return e, nil
}

func (e Extras) validate() error {
	if len(e.Beans) > MaxOpenBeans || len(e.Books) > MaxReading {
		return fmt.Errorf("at most %d open beans and %d open books", MaxOpenBeans, MaxReading)
	}
	if len(e.PastBeans) > MaxHistory || len(e.PastBooks) > MaxHistory {
		return fmt.Errorf("at most %d finished beans and %d finished books", MaxHistory, MaxHistory)
	}
	for _, list := range [][]Bean{e.Beans, e.PastBeans} {
		for _, b := range list {
			if err := required(b.Name, b.Origin, b.Roaster); err != nil {
				return fmt.Errorf("bean %q: %w", b.Name, err)
			}
			if err := checkText(b.Process); err != nil {
				return err
			}
		}
	}
	for _, list := range [][]Book{e.Books, e.PastBooks} {
		for _, b := range list {
			if err := required(b.Title, b.Author); err != nil {
				return fmt.Errorf("book %q: %w", b.Title, err)
			}
			if err := dates(b.Started, b.Finished); err != nil {
				return fmt.Errorf("book %q: %w", b.Title, err)
			}
		}
	}
	for _, list := range [][]Item{e.Gear, e.Bikes, e.SportGear, e.Music} {
		for _, it := range list {
			if err := required(append([]string{it.Kind, it.Name}, it.Details...)...); err != nil {
				return fmt.Errorf("item %q: %w", it.Name, err)
			}
		}
	}
	if err := required(e.Brews...); err != nil {
		return fmt.Errorf("brews: %w", err)
	}
	for _, p := range e.Podcasts {
		if err := required(p.Name); err != nil {
			return fmt.Errorf("podcast %q: %w", p.Name, err)
		}
		if err := checkText(p.Host); err != nil {
			return err
		}
	}
	if len(e.Gigs) > MaxGigs {
		return fmt.Errorf("at most %d gigs", MaxGigs)
	}
	for _, g := range e.Gigs {
		if err := required(g.Program, g.Place); err != nil {
			return fmt.Errorf("gig %q: %w", g.Program, err)
		}
		if err := checkText(g.Choir); err != nil {
			return err
		}
		if err := dates(g.Date); err != nil {
			return fmt.Errorf("gig %q: %w", g.Program, err)
		}
	}
	if len(e.Repertoire) > MaxRepertoire {
		return fmt.Errorf("at most %d pieces in the repertoire", MaxRepertoire)
	}
	for _, p := range e.Repertoire {
		if err := required(p.Title, p.Composer, p.Form); err != nil {
			return fmt.Errorf("piece %q: %w", p.Title, err)
		}
		if err := errors.Join(checkText(p.Scoring), checkText(p.Album)); err != nil {
			return fmt.Errorf("piece %q: %w", p.Title, err)
		}
		if p.Year < firstYear || p.Year > lastYear {
			return fmt.Errorf("piece %q: year %d is outside %d to %d", p.Title, p.Year, firstYear, lastYear)
		}
	}
	if e.Concerts < 0 {
		return errors.New("concerts cannot be negative")
	}
	for _, st := range e.Stages {
		if err := required(st.Title); err != nil {
			return fmt.Errorf("stage %q: %w", st.Title, err)
		}
		if err := errors.Join(checkText(st.Place), checkText(st.With)); err != nil {
			return fmt.Errorf("stage %q: %w", st.Title, err)
		}
		if st.Year != 0 && (st.Year < firstYear || st.Year > lastYear) {
			return fmt.Errorf("stage %q: year %d is outside %d to %d", st.Title, st.Year, firstYear, lastYear)
		}
	}
	if err := required(e.Countries...); err != nil {
		return fmt.Errorf("countries: %w", err)
	}
	return nil
}

func (e Extras) clone() Extras {
	return Extras{
		Beans:      append([]Bean{}, e.Beans...),
		PastBeans:  append([]Bean{}, e.PastBeans...),
		Gear:       append([]Item{}, e.Gear...),
		Brews:      append([]string{}, e.Brews...),
		Bikes:      append([]Item{}, e.Bikes...),
		SportGear:  append([]Item{}, e.SportGear...),
		Music:      append([]Item{}, e.Music...),
		Books:      append([]Book{}, e.Books...),
		PastBooks:  append([]Book{}, e.PastBooks...),
		Podcasts:   append([]Podcast{}, e.Podcasts...),
		Gigs:       append([]Gig{}, e.Gigs...),
		Repertoire: append([]Piece{}, e.Repertoire...),
		Concerts:   e.Concerts,
		Stages:     append([]Stage{}, e.Stages...),
		Countries:  append([]string{}, e.Countries...),
	}
}

func required(values ...string) error {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return errors.New("missing a required field")
		}
		if err := checkText(v); err != nil {
			return err
		}
	}
	return nil
}

func checkText(s string) error {
	if utf8.RuneCountInString(s) > MaxRunes {
		return fmt.Errorf("text longer than %d characters", MaxRunes)
	}
	if strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) || r == '<' || r == '>' }) {
		return fmt.Errorf("text %q contains control characters or angle brackets", s)
	}
	return nil
}

func dates(values ...string) error {
	for i, v := range values {
		if v == "" && i > 0 {
			continue
		}
		if _, err := time.Parse(dateLayout, v); err != nil {
			return fmt.Errorf("date %q is not YYYY-MM-DD", v)
		}
	}
	return nil
}

func decodeStrict(r io.Reader, v any) error {
	dec := json.NewDecoder(io.LimitReader(r, 256<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}
