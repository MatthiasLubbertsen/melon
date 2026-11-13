// Melon: Zermelo, but intuitive.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/matthiaslubbertsen/melon/api"
	"github.com/matthiaslubbertsen/melon/internal/core"
	"github.com/matthiaslubbertsen/melon/internal/secret"
	"github.com/matthiaslubbertsen/melon/internal/server"
	"github.com/matthiaslubbertsen/melon/internal/store"
	"github.com/matthiaslubbertsen/melon/internal/zermelo"
	"github.com/matthiaslubbertsen/melon/web"
)

var version = "dev"

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func truthy(key string) bool {
	switch strings.ToLower(env(key, "")) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func main() {
	if err := run(); err != nil {
		slog.Error("melon stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	addr := env("MELON_ADDR", ":8080")
	dataDir := env("MELON_DATA_DIR", "./data")
	demo := truthy("MELON_DEMO")

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	master, generated, err := secret.LoadMaster(os.Getenv("MELON_SECRET"), dataDir)
	if err != nil {
		return err
	}
	if generated {
		slog.Warn("MELON_SECRET is not set; generated a key in the data directory. Set MELON_SECRET in production so the key lives apart from the database.")
	}
	box, err := secret.NewBox(master)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(env("MELON_TIMEZONE", "Europe/Amsterdam"))
	if err != nil {
		return err
	}
	db, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	defer db.Close()

	up := zermelo.Router{Real: zermelo.NewHTTP()}
	if demo {
		up.Demo = zermelo.Demo{Loc: loc}
	}
	svc := &core.Service{Store: db, Up: up, Cache: zermelo.NewCache(), Box: box, Loc: loc, Demo: demo}
	srv := &server.Server{
		Core: svc, Store: db,
		PublicURL:  strings.TrimRight(env("MELON_PUBLIC_URL", ""), "/"),
		TrustProxy: truthy("MELON_TRUST_PROXY"),
		Demo:       demo, Version: version,
		Static: web.Dist(), OpenAPI: api.OpenAPI,
	}
	httpServer := &http.Server{
		Addr: addr, Handler: srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	slog.Info("melon is listening", "addr", addr, "demo", demo, "version", version)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
