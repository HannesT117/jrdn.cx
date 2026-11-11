package page

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/HannesT117/johannestrepesch.de/api/internal/extras"
	"github.com/HannesT117/johannestrepesch.de/api/internal/garmin"
)

type ActivitySource interface {
	Recent(ctx context.Context, now time.Time) ([]garmin.Activity, error)
}

type Publisher struct {
	Shell    string
	Extras   string
	Out      string
	Activity ActivitySource
	Zone     *time.Location
}

func (p *Publisher) Publish(ctx context.Context, now time.Time) error {
	shell, err := os.ReadFile(p.Shell)
	if err != nil {
		return err
	}
	e, err := extras.Load(p.Extras)
	if err != nil {
		return err
	}
	d := Data{Extras: e, Now: now, Zone: p.Zone}
	if p.Activity != nil {
		acts, err := p.Activity.Recent(ctx, now)
		if err != nil {
			slog.Warn("garmin", "err", err)
		}
		d.Activities, d.ActivityKnown = acts, acts != nil
	}
	out, err := Render(shell, d)
	if err != nil {
		return err
	}
	return write(p.Out, out)
}

func (p *Publisher) Run(ctx context.Context, every time.Duration) {
	publish := func() {
		if err := p.Publish(ctx, time.Now()); err != nil {
			slog.Error("page not published", "err", err)
		}
	}
	publish()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			publish()
		}
	}
}

func write(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".page-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
