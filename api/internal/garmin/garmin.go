package garmin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	Window        = 96 * time.Hour
	CacheFor      = 15 * time.Minute
	RetryAfter    = time.Minute
	precision     = 15 * time.Minute
	refreshWindow = time.Hour
	baseURL       = "https://personal.connectapi.garmin.com"
	tokenPath     = "/personalapitoken-service/personal-api/personal-api-key/token"
	searchPath    = "/activitylist-service/activities/search/activities"
	startLayout   = time.DateTime
)

type Activity struct {
	Kind    string    `json:"kind"`
	Start   time.Time `json:"start"`
	Minutes int       `json:"minutes"`
}

type Client struct {
	http *http.Client
	key  string

	mu         sync.Mutex
	access     string
	expires    time.Time
	activities []Activity
	fresh      time.Time
	retryAt    time.Time
	lastErr    error
}

func New(key string, transport http.RoundTripper) *Client {
	if transport == nil {
		transport = &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
			TLSHandshakeTimeout: 3 * time.Second,
		}
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 10 * time.Second}, key: key}
}

func (c *Client) Recent(ctx context.Context, now time.Time) ([]Activity, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.Before(c.fresh) {
		return within(c.activities, now), nil
	}
	if now.Before(c.retryAt) {
		return within(c.activities, now), c.lastErr
	}
	activities, err := c.fetch(ctx, now)
	if err != nil {
		c.retryAt, c.lastErr = now.Add(RetryAfter), err
		return within(c.activities, now), err
	}
	c.activities, c.fresh = activities, now.Add(CacheFor)
	return within(activities, now), nil
}

func (c *Client) fetch(ctx context.Context, now time.Time) ([]Activity, error) {
	if c.access == "" || !now.Before(c.expires.Add(-refreshWindow)) {
		if err := c.mint(ctx); err != nil {
			return nil, err
		}
	}
	query := url.Values{
		"start":     {"0"},
		"limit":     {"50"},
		"startDate": {now.Add(-Window - 24*time.Hour).UTC().Format(time.DateOnly)},
		"endDate":   {now.Add(24 * time.Hour).UTC().Format(time.DateOnly)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+searchPath+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.access)
	req.Header.Set("Accept", "application/json")
	body, err := c.do(req)
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			c.access = ""
		}
		return nil, err
	}
	var raw []struct {
		StartTimeGMT *string  `json:"startTimeGMT"`
		Duration     *float64 `json:"duration"`
		ActivityType struct {
			TypeKey string `json:"typeKey"`
		} `json:"activityType"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("garmin activities: %w", err)
	}
	activities := make([]Activity, 0, len(raw))
	for _, a := range raw {
		if a.StartTimeGMT == nil || a.Duration == nil {
			return nil, errors.New("garmin activity without startTimeGMT or duration")
		}
		start, err := time.ParseInLocation(startLayout, *a.StartTimeGMT, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("garmin activity start: %w", err)
		}
		minutes := int(math.Round(*a.Duration/precision.Seconds())) * int(precision.Minutes())
		activities = append(activities, Activity{
			Kind:    kind(a.ActivityType.TypeKey),
			Start:   start.Truncate(precision),
			Minutes: max(minutes, int(precision.Minutes())),
		})
	}
	slices.SortFunc(activities, func(a, b Activity) int { return b.Start.Compare(a.Start) })
	return activities, nil
}

func (c *Client) mint(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+tokenPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "ApiKey "+c.key)
	req.Header.Set("Accept", "*/*")
	body, err := c.do(req)
	if errors.Is(err, errUnauthorized) {
		return errors.New("garmin rejected the personal API key, it may have expired")
	}
	if err != nil {
		return fmt.Errorf("garmin token: %w", err)
	}
	token := strings.TrimSpace(string(body))
	expires, err := expiry(token)
	if err != nil {
		return err
	}
	c.access, c.expires = token, expires
	return nil
}

func expiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("garmin token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, fmt.Errorf("garmin token payload: %w", err)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, errors.New("garmin token has no exp claim")
	}
	return time.Unix(claims.Exp, 0), nil
}

var errUnauthorized = errors.New("garmin rejected the access token")

func (c *Client) do(req *http.Request) ([]byte, error) {
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, errUnauthorized
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("garmin answered %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 1<<20))
}

func within(activities []Activity, now time.Time) []Activity {
	if activities == nil {
		return nil
	}
	from := now.Add(-Window)
	out := []Activity{}
	for _, a := range activities {
		if !a.Start.Before(from) {
			out = append(out, a)
		}
	}
	return out
}

func kind(typeKey string) string {
	switch {
	case strings.Contains(typeKey, "running"):
		return "run"
	case strings.Contains(typeKey, "cycling"), strings.Contains(typeKey, "biking"), strings.Contains(typeKey, "ride"), strings.HasPrefix(typeKey, "e_bike"):
		return "ride"
	case strings.Contains(typeKey, "swimming"):
		return "swim"
	}
	return "other"
}
