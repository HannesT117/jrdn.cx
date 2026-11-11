package garmin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2031, 3, 3, 9, 0, 0, 0, time.UTC)

type fakeGarmin struct {
	tokenCalls    int
	activityCalls int
	query         map[string]string
	keySent       string
	expires       time.Time
	activities    string
	failAPI       bool
	reject        bool
}

func jwt(exp time.Time) string {
	payload, _ := json.Marshal(map[string]int64{"exp": exp.Unix()})
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func (f *fakeGarmin) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/personalapitoken-service/personal-api/personal-api-key/token":
		f.tokenCalls++
		f.keySent = req.Header.Get("Authorization")
		if req.Method != http.MethodPost || f.keySent != "ApiKey key-1" {
			return respond(http.StatusUnauthorized, ``), nil
		}
		return respond(http.StatusOK, jwt(f.expires)+"\n"), nil
	case "/activitylist-service/activities/search/activities":
		f.activityCalls++
		f.query = map[string]string{}
		for k, v := range req.URL.Query() {
			f.query[k] = v[0]
		}
		if f.reject || req.Header.Get("Authorization") != "Bearer "+jwt(f.expires) {
			f.reject = false
			return respond(http.StatusUnauthorized, `{}`), nil
		}
		if f.failAPI {
			return respond(http.StatusInternalServerError, `{}`), nil
		}
		return respond(http.StatusOK, f.activities), nil
	}
	return respond(http.StatusNotFound, `{}`), nil
}

func respond(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

const twoActivities = `[
	{"activityId":1,"activityName":"Secret loop","startTimeGMT":"2031-03-01 21:07:00","startTimeLocal":"2031-03-02 08:07:00","duration":7300.4,"distance":61234.5,"startLatitude":-32.9,"activityType":{"typeKey":"gravel_cycling","typeId":143}},
	{"activityId":2,"startTimeGMT":"2031-03-02 19:52:00","duration":2700,"distance":9100,"activityType":{"typeKey":"trail_running"}}
]`

func newClient(fake *fakeGarmin) *Client {
	if fake.expires.IsZero() {
		fake.expires = now.Add(24 * time.Hour)
	}
	return New("key-1", fake)
}

func TestRecentReturnsCoarseActivitiesNewestFirst(t *testing.T) {
	fake := &fakeGarmin{activities: twoActivities}
	got, err := newClient(fake).Recent(context.Background(), now)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	want := []Activity{
		{Kind: "run", Start: time.Date(2031, 3, 2, 19, 45, 0, 0, time.UTC), Minutes: 45},
		{Kind: "ride", Start: time.Date(2031, 3, 1, 21, 0, 0, 0, time.UTC), Minutes: 120},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v", got)
	}
	if fake.query["startDate"] != "2031-02-26" || fake.query["endDate"] != "2031-03-04" || fake.query["limit"] != "50" || fake.query["start"] != "0" {
		t.Errorf("query %v", fake.query)
	}
}

func TestRecentOnlyExposesKindStartAndMinutes(t *testing.T) {
	got, _ := newClient(&fakeGarmin{activities: twoActivities}).Recent(context.Background(), now)
	raw, _ := json.Marshal(got)
	for _, leak := range []string{"Secret", "61234", "32.9", "distance", "Name", "08:07"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("payload leaks %q: %s", leak, raw)
		}
	}
}

func TestRecentCachesAndReusesTheAccessToken(t *testing.T) {
	fake := &fakeGarmin{activities: twoActivities}
	c := newClient(fake)
	for _, at := range []time.Time{now, now.Add(CacheFor - time.Second), now.Add(CacheFor + time.Second)} {
		if _, err := c.Recent(context.Background(), at); err != nil {
			t.Fatal(err)
		}
	}
	if fake.activityCalls != 2 || fake.tokenCalls != 1 {
		t.Fatalf("activity calls %d, token calls %d", fake.activityCalls, fake.tokenCalls)
	}
}

func TestRecentMintsANewTokenBeforeTheOldOneExpires(t *testing.T) {
	fake := &fakeGarmin{activities: `[]`, expires: now.Add(time.Hour)}
	c := newClient(fake)
	if _, err := c.Recent(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Recent(context.Background(), now.Add(CacheFor+time.Second)); err != nil {
		t.Fatal(err)
	}
	if fake.tokenCalls != 2 {
		t.Fatalf("token calls %d", fake.tokenCalls)
	}
}

func TestRecentMintsANewTokenAfterARejection(t *testing.T) {
	fake := &fakeGarmin{activities: `[]`}
	c := newClient(fake)
	if _, err := c.Recent(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	fake.reject = true
	if _, err := c.Recent(context.Background(), now.Add(CacheFor+time.Second)); err == nil {
		t.Fatal("expected the rejection to surface")
	}
	if _, err := c.Recent(context.Background(), now.Add(CacheFor+RetryAfter+2*time.Second)); err != nil || fake.tokenCalls != 2 {
		t.Fatalf("token calls %d, %v", fake.tokenCalls, err)
	}
}

func TestRecentReportsARejectedKey(t *testing.T) {
	c := New("wrong", &fakeGarmin{})
	if _, err := c.Recent(context.Background(), now); err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("got %v", err)
	}
}

func TestRecentServesStaleDataWhenGarminFails(t *testing.T) {
	fake := &fakeGarmin{activities: twoActivities}
	c := newClient(fake)
	ctx := context.Background()
	if _, err := c.Recent(ctx, now); err != nil {
		t.Fatal(err)
	}
	fake.failAPI = true
	got, err := c.Recent(ctx, now.Add(CacheFor+time.Second))
	if err == nil || len(got) != 2 {
		t.Fatalf("got %+v, %v", got, err)
	}
	calls := fake.activityCalls
	if _, err := c.Recent(ctx, now.Add(CacheFor+30*time.Second)); err == nil || fake.activityCalls != calls {
		t.Errorf("retried too soon: %d calls", fake.activityCalls)
	}
}

func TestRecentDropsActivitiesThatLeftTheWindow(t *testing.T) {
	got, err := newClient(&fakeGarmin{activities: twoActivities}).Recent(context.Background(), time.Date(2031, 3, 5, 22, 0, 0, 0, time.UTC))
	if err != nil || len(got) != 1 || got[0].Kind != "run" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestRecentRejectsAnUnexpectedShape(t *testing.T) {
	for name, body := range map[string]string{
		"no start":    `[{"duration":60,"activityType":{"typeKey":"running"}}]`,
		"no duration": `[{"startTimeGMT":"2031-03-02 19:52:00","activityType":{"typeKey":"running"}}]`,
		"bad start":   `[{"startTimeGMT":"yesterday","duration":60,"activityType":{"typeKey":"running"}}]`,
		"not a list":  `{"activities":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := newClient(&fakeGarmin{activities: body}).Recent(context.Background(), now); err == nil {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestKindGroupsActivityTypes(t *testing.T) {
	for typeKey, want := range map[string]string{
		"running": "run", "trail_running": "run", "treadmill_running": "run", "track_running": "run",
		"cycling": "ride", "road_biking": "ride", "gravel_cycling": "ride", "mountain_biking": "ride", "indoor_cycling": "ride", "virtual_ride": "ride", "e_bike_fitness": "ride",
		"lap_swimming": "swim", "open_water_swimming": "swim",
		"strength_training": "other", "yoga": "other", "": "other",
	} {
		if got := kind(typeKey); got != want {
			t.Errorf("kind(%q) = %q, want %q", typeKey, got, want)
		}
	}
}

func TestRecentReturnsNilWhenItNeverHadData(t *testing.T) {
	got, err := newClient(&fakeGarmin{failAPI: true}).Recent(context.Background(), now)
	if err == nil || got != nil {
		t.Fatalf("got %#v, %v", got, err)
	}
}
