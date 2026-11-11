package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/HannesT117/johannestrepesch.de/api/internal/extras"
)

type ExtrasSource interface {
	Get() (extras.Extras, error)
}

func NewExtras(source ExtrasSource, zone *time.Location) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e, err := source.Get()
		if err != nil {
			slog.Warn("extras", "err", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(e.Upcoming(time.Now().In(zone)))
	})
}
