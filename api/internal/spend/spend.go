package spend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	CheckEvery = time.Minute
	TrustFor   = 10 * time.Minute
	baseURL    = "https://api.opper.ai"
)

var ErrOverBudget = errors.New("daily budget spent")

type Guard struct {
	http    *http.Client
	key     string
	project string
	limit   float64

	mu        sync.Mutex
	uuid      string
	day       time.Time
	spent     float64
	checked   time.Time
	confirmed time.Time
	lastErr   error
}

func New(key, project string, limitUSD float64, transport http.RoundTripper) *Guard {
	if transport == nil {
		transport = &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
			TLSHandshakeTimeout: 3 * time.Second,
		}
	}
	return &Guard{http: &http.Client{Transport: transport, Timeout: 5 * time.Second}, key: key, project: project, limit: limitUSD}
}

func (g *Guard) Allow(ctx context.Context, now time.Time) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	day := now.UTC().Truncate(24 * time.Hour)
	if !day.Equal(g.day) {
		g.day, g.spent, g.checked, g.confirmed = day, 0, time.Time{}, time.Time{}
	}
	if g.checked.IsZero() || !now.Before(g.checked.Add(CheckEvery)) {
		g.checked = now
		if spent, err := g.fetch(ctx, day); err != nil {
			g.lastErr = err
		} else {
			g.spent, g.confirmed, g.lastErr = spent, now, nil
		}
	}
	if g.confirmed.IsZero() || !now.Before(g.confirmed.Add(TrustFor)) {
		return fmt.Errorf("opper usage unknown: %w", g.lastErr)
	}
	if g.spent >= g.limit {
		return ErrOverBudget
	}
	return nil
}

func (g *Guard) fetch(ctx context.Context, day time.Time) (float64, error) {
	if g.uuid == "" {
		var projects struct {
			Data []struct {
				UUID string `json:"uuid"`
				Name string `json:"name"`
			} `json:"data"`
		}
		if err := g.get(ctx, "/management/v1/projects", nil, &projects); err != nil {
			return 0, err
		}
		for _, p := range projects.Data {
			if p.Name == g.project {
				g.uuid = p.UUID
			}
		}
		if g.uuid == "" {
			return 0, fmt.Errorf("opper has no project %q", g.project)
		}
	}
	var usage struct {
		Data []struct {
			TotalCost float64 `json:"total_cost"`
		} `json:"data"`
	}
	query := url.Values{"from": {day.Format(time.RFC3339)}, "project_uuid": {g.uuid}}
	if err := g.get(ctx, "/management/v1/usage", query, &usage); err != nil {
		return 0, err
	}
	var total float64
	for _, row := range usage.Data {
		total += row.TotalCost
	}
	return total, nil
}

func (g *Guard) get(ctx context.Context, path string, query url.Values, into any) error {
	target := baseURL + path
	if query != nil {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.key)
	req.Header.Set("Accept", "application/json")
	res, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("opper %s answered %s", path, res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(into)
}
