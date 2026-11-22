// Command labserver runs the mBot2 Agent Lab: embedded MQTT broker, robot
// registry, agents, model access, storage and web UI in a single binary.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/broker"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/web"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configPath := flag.String("config", "configs/lab.yaml", "path to the lab configuration")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	if err := run(*configPath, log); err != nil {
		log.Error("labserver stopped", "err", err)
		os.Exit(1)
	}
}

func run(configPath string, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.Server.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	var tlsConfig *tls.Config
	var certManager *autocert.Manager
	if cfg.Server.TLS.Mode == config.TLSAutocert {
		certManager = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.Server.TLS.Hosts...),
			Cache:      autocert.DirCache(cacheDir(cfg)),
		}
		tlsConfig = certManager.TLSConfig()
	}

	passwords := map[string]string{}
	specs := make([]robots.Spec, 0, len(cfg.Robots))
	for _, r := range cfg.Robots {
		passwords[r.ID] = r.Password
		specs = append(specs, robots.Spec{ID: r.ID, Color: r.Color})
	}
	mq, err := broker.New(broker.Options{
		PlainAddr: cfg.Server.MQTTPlainAddr,
		TLSAddr:   cfg.Server.MQTTTLSAddr,
		TLSConfig: tlsConfig,
		Robots:    passwords,
		Logger:    log,
	})
	if err != nil {
		return err
	}
	if err := mq.Serve(); err != nil {
		return fmt.Errorf("start broker: %w", err)
	}
	defer mq.Close()

	reg := robots.New(mq, specs, log)
	for robotID, a := range cfg.Assignments() {
		team, _ := cfg.Team(a.TeamID)
		if err := reg.Assign(robotID, a.TeamID, team.Name, a.Alias); err != nil {
			return err
		}
	}
	go func() {
		if err := reg.Run(ctx); err != nil {
			log.Error("robot registry", "err", err)
			stop()
		}
	}()

	ui := &web.Server{Config: cfg, Robots: reg, Store: db, Hub: web.NewHub(), Log: log}
	go ui.Run(ctx)
	srv := &http.Server{
		Addr:              cfg.Server.HTTPAddr,
		Handler:           ui.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         tlsConfig,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("labserver started", "version", version, "http", cfg.Server.HTTPAddr,
			"mqtt", cfg.Server.MQTTPlainAddr, "mqtts", cfg.Server.MQTTTLSAddr, "profile", cfg.Profile)
		if tlsConfig != nil {
			errc <- srv.ListenAndServeTLS("", "")
		} else {
			errc <- srv.ListenAndServe()
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func cacheDir(cfg *config.Config) string {
	if cfg.Server.TLS.CacheDir != "" {
		return cfg.Server.TLS.CacheDir
	}
	return "autocert-cache"
}
