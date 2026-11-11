package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HannesT117/johannestrepesch.de/api/internal/extras"
)

type fakeExtras struct {
	e   extras.Extras
	err error
}

func (f fakeExtras) Get() (extras.Extras, error) { return f.e, f.err }

func getExtras(t *testing.T, source ExtrasSource) (*httptest.ResponseRecorder, extras.Extras) {
	t.Helper()
	rec := httptest.NewRecorder()
	NewExtras(source, time.UTC).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/extras", nil))
	var e extras.Extras
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("response is not JSON: %q", rec.Body.String())
	}
	return rec, e
}

func TestExtrasServesTheCurrentData(t *testing.T) {
	rec, got := getExtras(t, fakeExtras{e: extras.Extras{Beans: []extras.Bean{{Name: "Lumen"}}}})
	if rec.Code != http.StatusOK || len(got.Beans) != 1 || got.Beans[0].Name != "Lumen" {
		t.Fatalf("got %d %+v", rec.Code, got)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
}

func TestExtrasKeepsServingTheLastGoodVersion(t *testing.T) {
	rec, got := getExtras(t, fakeExtras{e: extras.Extras{Beans: []extras.Bean{{Name: "Lumen"}}}, err: errors.Join(extras.ErrStale, errors.New("bad json"))})
	if rec.Code != http.StatusOK || len(got.Beans) != 1 {
		t.Fatalf("got %d %+v", rec.Code, got)
	}
}

func TestExtrasHidesGigsThatHavePassed(t *testing.T) {
	_, got := getExtras(t, fakeExtras{e: extras.Extras{Gigs: []extras.Gig{
		{Date: "2001-01-01", Program: "Old", Place: "Hall"},
		{Date: "2999-01-01", Program: "Future", Place: "Hall"},
	}}})
	if len(got.Gigs) != 1 || got.Gigs[0].Program != "Future" {
		t.Fatalf("got %+v", got.Gigs)
	}
}
