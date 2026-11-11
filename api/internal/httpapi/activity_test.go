package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HannesT117/johannestrepesch.de/api/internal/garmin"
)

type fakeActivity struct {
	activities []garmin.Activity
	err        error
}

func (f fakeActivity) Recent(context.Context, time.Time) ([]garmin.Activity, error) {
	return f.activities, f.err
}

func getActivity(t *testing.T, source ActivitySource) (*httptest.ResponseRecorder, map[string][]garmin.Activity) {
	t.Helper()
	rec := httptest.NewRecorder()
	NewActivity(source).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/activity", nil))
	var body map[string][]garmin.Activity
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %q", rec.Body.String())
	}
	return rec, body
}

func TestActivityServesRecentActivities(t *testing.T) {
	rec, body := getActivity(t, fakeActivity{activities: []garmin.Activity{{Kind: "run", Minutes: 45}}})
	if rec.Code != http.StatusOK || len(body["activities"]) != 1 || rec.Header().Get("Cache-Control") != "public, max-age=600" {
		t.Fatalf("got %d %+v %q", rec.Code, body, rec.Header().Get("Cache-Control"))
	}
}

func TestActivityServesStaleDataButNotForLong(t *testing.T) {
	rec, body := getActivity(t, fakeActivity{activities: []garmin.Activity{{Kind: "ride"}}, err: errors.New("garmin down")})
	if rec.Code != http.StatusOK || len(body["activities"]) != 1 || rec.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("got %d %+v %q", rec.Code, body, rec.Header().Get("Cache-Control"))
	}
}

func TestActivityIsUnavailableWithoutDataOrConfig(t *testing.T) {
	for name, source := range map[string]ActivitySource{
		"not configured": nil,
		"failing":        fakeActivity{err: errors.New("garmin down")},
	} {
		t.Run(name, func(t *testing.T) {
			if rec, _ := getActivity(t, source); rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Cache-Control"))
			}
		})
	}
}
