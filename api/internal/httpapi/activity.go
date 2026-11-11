package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/HannesT117/johannestrepesch.de/api/internal/garmin"
)

type ActivitySource interface {
	Recent(ctx context.Context, now time.Time) ([]garmin.Activity, error)
}

func NewActivity(source ActivitySource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		var activities []garmin.Activity
		var err error
		if source != nil {
			ctx, cancel := context.WithTimeout(r.Context(), ModelTimeout)
			defer cancel()
			activities, err = source.Recent(ctx, time.Now())
			if err != nil {
				slog.Warn("garmin", "err", err)
			}
		}
		switch {
		case source == nil || (err != nil && activities == nil):
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
			activities = []garmin.Activity{}
		case err != nil:
			w.Header().Set("Cache-Control", "public, max-age=60")
		default:
			w.Header().Set("Cache-Control", "public, max-age=600")
		}
		_ = json.NewEncoder(w).Encode(map[string][]garmin.Activity{"activities": activities})
	})
}
