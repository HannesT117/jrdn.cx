package page

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HannesT117/johannestrepesch.de/api/internal/garmin"
)

type fakeActivity struct {
	acts []garmin.Activity
	err  error
}

func (f fakeActivity) Recent(context.Context, time.Time) ([]garmin.Activity, error) {
	return f.acts, f.err
}

func publisher(t *testing.T, extrasJSON string, activity ActivitySource) *Publisher {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shell.html"), []byte(shell), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extras.json"), []byte(extrasJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Publisher{
		Shell:    filepath.Join(dir, "shell.html"),
		Extras:   filepath.Join(dir, "extras.json"),
		Out:      filepath.Join(dir, "html", "index.html"),
		Activity: activity,
		Zone:     sydney,
	}
}

func TestPublishWritesAReadablePage(t *testing.T) {
	p := publisher(t, `{"brews":["V60"]}`, nil)
	if err := os.MkdirAll(filepath.Dir(p.Out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(context.Background(), now); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	raw, err := os.ReadFile(p.Out)
	if err != nil || !strings.Contains(string(raw), "<li>V60</li>") {
		t.Fatalf("got %q, %v", raw, err)
	}
	info, _ := os.Stat(p.Out)
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
}

func TestPublishKeepsTheOldPageWhenTheExtrasBreak(t *testing.T) {
	p := publisher(t, `{"beans":[{"name":"","origin":"P","roaster":"R"}]}`, nil)
	if err := os.MkdirAll(filepath.Dir(p.Out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Out, []byte("old page"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(context.Background(), now); err == nil {
		t.Fatal("expected an error")
	}
	if raw, _ := os.ReadFile(p.Out); string(raw) != "old page" {
		t.Fatalf("page replaced with %q", raw)
	}
}

func TestPublishShowsStaleActivityAndHidesUnknownActivity(t *testing.T) {
	stale := fakeActivity{acts: []garmin.Activity{{Kind: "run", Start: now.Add(-3 * time.Hour), Minutes: 45}}, err: errors.New("garmin down")}
	p := publisher(t, `{}`, stale)
	if err := os.MkdirAll(filepath.Dir(p.Out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(p.Out); !strings.Contains(string(raw), `class="track"`) {
		t.Fatal("stale activity not shown")
	}
	p.Activity = fakeActivity{err: errors.New("garmin down")}
	if err := p.Publish(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(p.Out); strings.Contains(string(raw), `class="track"`) {
		t.Fatal("unknown activity rendered a timeline")
	}
}

func TestPublishFailsWithoutAShell(t *testing.T) {
	p := publisher(t, `{}`, nil)
	p.Shell = filepath.Join(t.TempDir(), "missing.html")
	if err := p.Publish(context.Background(), now); err == nil {
		t.Fatal("expected an error")
	}
}
