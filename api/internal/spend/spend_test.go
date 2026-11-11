package spend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2031, 3, 3, 9, 30, 0, 0, time.UTC)

const projectUUID = "0b6f7c2e-9a51-4f3e-8d8a-2f1c5e7a9b10"

type fakeOpper struct {
	projectCalls int
	usageCalls   int
	query        map[string]string
	auth         string
	cost         float64
	failUsage    bool
}

func (f *fakeOpper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.auth = req.Header.Get("Authorization")
	switch req.URL.Path {
	case "/management/v1/projects":
		f.projectCalls++
		return respond(http.StatusOK, `{"data":[{"uuid":"1d2c","name":"Other","description":""},{"uuid":"`+projectUUID+`","name":"Portfolio","description":""}],"meta":{"total_count":2}}`), nil
	case "/management/v1/usage":
		f.usageCalls++
		f.query = map[string]string{}
		for k, v := range req.URL.Query() {
			f.query[k] = v[0]
		}
		if f.failUsage {
			return respond(http.StatusInternalServerError, `{}`), nil
		}
		return respond(http.StatusOK, fmt.Sprintf(`{"data":[{"count":12,"cost":%[1]g,"evals_cost":0,"guards_cost":0,"total_cost":%[1]g}],"meta":{"currency":"USD"}}`, f.cost)), nil
	}
	return respond(http.StatusNotFound, `{}`), nil
}

func respond(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

func newGuard(fake *fakeOpper) *Guard {
	return New("op-mak-test", "Portfolio", 2, fake)
}

func TestAllowsUnderTheDailyBudget(t *testing.T) {
	fake := &fakeOpper{cost: 1.25}
	if err := newGuard(fake).Allow(context.Background(), now); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if fake.auth != "Bearer op-mak-test" {
		t.Errorf("auth %q", fake.auth)
	}
	if fake.query["project_uuid"] != projectUUID || fake.query["from"] != "2031-03-03T00:00:00Z" {
		t.Errorf("query %v", fake.query)
	}
}

func TestRefusesOnceTheBudgetIsSpent(t *testing.T) {
	for _, cost := range []float64{2, 2.4} {
		if err := newGuard(&fakeOpper{cost: cost}).Allow(context.Background(), now); !errors.Is(err, ErrOverBudget) {
			t.Errorf("cost %v: got %v", cost, err)
		}
	}
}

func TestAsksOpperAtMostOncePerInterval(t *testing.T) {
	fake := &fakeOpper{cost: 0.5}
	g := newGuard(fake)
	for _, at := range []time.Time{now, now.Add(CheckEvery - time.Second), now.Add(CheckEvery + time.Second)} {
		if err := g.Allow(context.Background(), at); err != nil {
			t.Fatal(err)
		}
	}
	if fake.usageCalls != 2 || fake.projectCalls != 1 {
		t.Fatalf("usage calls %d, project calls %d", fake.usageCalls, fake.projectCalls)
	}
}

func TestKeepsUsingARecentFigureWhenOpperFails(t *testing.T) {
	fake := &fakeOpper{cost: 0.5}
	g := newGuard(fake)
	if err := g.Allow(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	fake.failUsage = true
	if err := g.Allow(context.Background(), now.Add(TrustFor-time.Second)); err != nil {
		t.Fatalf("recent figure: %v", err)
	}
	if err := g.Allow(context.Background(), now.Add(TrustFor+time.Second)); err == nil || errors.Is(err, ErrOverBudget) {
		t.Fatalf("stale figure: %v", err)
	}
}

func TestRefusesWithoutAnyFigure(t *testing.T) {
	if err := newGuard(&fakeOpper{failUsage: true}).Allow(context.Background(), now); err == nil {
		t.Fatal("expected a refusal")
	}
}

func TestStartsAfreshAtUTCMidnight(t *testing.T) {
	fake := &fakeOpper{cost: 3}
	g := newGuard(fake)
	if err := g.Allow(context.Background(), time.Date(2031, 3, 3, 23, 59, 30, 0, time.UTC)); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("before midnight: %v", err)
	}
	fake.cost = 0
	if err := g.Allow(context.Background(), time.Date(2031, 3, 4, 0, 0, 10, 0, time.UTC)); err != nil {
		t.Fatalf("after midnight: %v", err)
	}
	if fake.query["from"] != "2031-03-04T00:00:00Z" {
		t.Errorf("from %q", fake.query["from"])
	}
}

func TestRefusesWhenTheProjectIsUnknown(t *testing.T) {
	if err := New("op-mak-test", "Missing", 2, &fakeOpper{}).Allow(context.Background(), now); err == nil || !strings.Contains(err.Error(), "Missing") {
		t.Fatalf("got %v", err)
	}
}
