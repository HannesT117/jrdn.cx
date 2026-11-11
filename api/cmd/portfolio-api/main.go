package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/HannesT117/johannestrepesch.de/api/internal/config"
	"github.com/HannesT117/johannestrepesch.de/api/internal/content"
	"github.com/HannesT117/johannestrepesch.de/api/internal/extras"
	"github.com/HannesT117/johannestrepesch.de/api/internal/garmin"
	"github.com/HannesT117/johannestrepesch.de/api/internal/httpapi"
	"github.com/HannesT117/johannestrepesch.de/api/internal/limit"
	"github.com/HannesT117/johannestrepesch.de/api/internal/model"
)

func main() {
	envPath := flag.String("env", "portfolio-api.env", "path to the env file")
	contentPath := flag.String("content", "portfolio-content.json", "path to the content file")
	extrasPath := flag.String("extras", "portfolio-extras.json", "path to the extras file")
	check := flag.Bool("check", false, "validate the env, content and extras files and exit")
	zone := flag.String("tz", "Australia/Sydney", "time zone for dates in the extras file")
	flag.Parse()

	if *check {
		if _, err := config.Load(*envPath); err != nil {
			slog.Error("env invalid", "err", err)
			os.Exit(1)
		}
		if _, err := content.Load(*contentPath); err != nil {
			slog.Error("content invalid", "err", err)
			os.Exit(1)
		}
		if _, err := extras.Load(*extrasPath); err != nil {
			slog.Error("extras invalid", "err", err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.Load(*envPath)
	if err != nil {
		slog.Error("config invalid, running disabled", "err", err)
	}
	c, err := content.Load(*contentPath)
	if err != nil {
		slog.Error("content unavailable, running disabled", "err", err)
		cfg.Enabled = false
	}

	limiter := limit.New([]byte(cfg.HMACSecret), cfg.CapPerClient, cfg.CapGlobal)
	handler := http.NewServeMux()
	handler.Handle("/api/", httpapi.New(
		httpapi.Config{Enabled: cfg.Enabled, Origins: cfg.Origins, ClientIPHeader: cfg.ClientIPHeader},
		c,
		model.NewClient(cfg.OpperKey),
		limiter,
	))
	loc, err := time.LoadLocation(*zone)
	if err != nil {
		slog.Error("unknown time zone, using UTC", "zone", *zone, "err", err)
		loc = time.UTC
	}
	handler.Handle("GET /api/extras", httpapi.NewExtras(extras.NewStore(*extrasPath), loc))
	var activity httpapi.ActivitySource
	if cfg.GarminAPIKey != "" {
		activity = garmin.New(cfg.GarminAPIKey, nil)
	}
	handler.Handle("GET /api/activity", httpapi.NewActivity(activity))

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      httpapi.ModelTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				limiter.Sweep(now)
			case <-ctx.Done():
				return
			}
		}
	}()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), httpapi.ModelTimeout+2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	slog.Info("listening", "addr", srv.Addr, "enabled", cfg.Enabled)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
	<-stopped
}
