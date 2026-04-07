package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tunnel-ops/tunnel/internal/names"
	"github.com/tunnel-ops/tunnel/internal/proxy"
)

// Version and BuildTime are injected at build time via -ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

var startTime = time.Now()

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("starting", "version", Version, "built", BuildTime)

	store, err := names.New("")
	if err != nil {
		slog.Error("failed to initialize names store", "error", err)
		os.Exit(1)
	}

	cfg, err := proxy.LoadConfig(store)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	// Health server on a separate port so it is never routed through the
	// proxy's Host-based dispatch (a request to localhost:HEALTH_PORT would
	// fail domain validation otherwise).
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":%q,"uptime":%q}`,
			Version, time.Since(startTime).Round(time.Second).String())
	})
	healthSrv := &http.Server{
		Addr:         ":" + cfg.HealthPort,
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server error", "error", err)
		}
	}()

	h := proxy.New(cfg)

	srv := &http.Server{
		Addr:         ":" + cfg.ProxyPort,
		Handler:      h,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	go func() {
		slog.Info("proxy listening", "port", cfg.ProxyPort, "domain", cfg.Domain, "health_port", cfg.HealthPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	slog.Info("shutting down", "drain_timeout", "30s")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("proxy shutdown error", "error", err)
	}
	if err := healthSrv.Shutdown(ctx); err != nil {
		slog.Error("health server shutdown error", "error", err)
	}
	slog.Info("stopped")
}
