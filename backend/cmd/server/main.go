package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
	"github.com/waifu-im/waifu-api/backend/internal/db"
	"github.com/waifu-im/waifu-api/backend/internal/redisx"
	"github.com/waifu-im/waifu-api/backend/internal/server"
	"github.com/waifu-im/waifu-api/backend/internal/storage"
)

func main() {
	envPath := flag.String("env", "", "path to the .env file (auto-detected when empty)")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	var level slog.LevelVar
	level.Set(parseLogLevel(os.Getenv("LOG_LEVEL")))
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &level}))
	slog.SetDefault(log)

	cfg, err := config.Load(*envPath)
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	level.Set(parseLogLevel(os.Getenv("LOG_LEVEL")))
	log.Info("configuration loaded", "env", *envPath, "environment", cfg.Environment)
	cfg.LogSummary(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	instance := cfg.Cluster.InstanceID
	log = log.With("instance", instance)
	pool, err := db.Connect(ctx, cfg.DBURL(), db.Options{
		MaxConns:         cfg.DBMaxConns,
		StatementTimeout: cfg.DBStatementTimeout,
		ApplicationName:  "waifu-api/" + instance,
	})
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	lockPool, err := db.Connect(ctx, cfg.DBURL(), db.Options{
		MaxConns:        cfg.Cluster.LockConnections,
		MinConns:        1,
		ApplicationName: "waifu-api/" + instance + "/locks",
		KeepAlive:       true,
	})
	if err != nil {
		log.Error("database connection for cluster locks failed", "error", err)
		os.Exit(1)
	}
	log.Info("database connected", "host", cfg.DBHost, "db", cfg.DBName)

	opts := []server.Option{server.WithInstanceID(instance), server.WithLockPool(lockPool)}
	if cfg.Cluster.RedisURL != "" {
		rc, err := redisx.New(cfg.Cluster.RedisURL, redisx.Options{PoolSize: 32})
		if err != nil {
			log.Error("invalid REDIS_URL", "error", err)
			os.Exit(1)
		}
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if err := rc.Ping(pingCtx); err != nil {
			log.Warn("shared rate limit store not reachable yet, limiting per instance until it is", "error", err)
		} else {
			log.Info("shared rate limit store connected", "addr", rc.Addr())
		}
		cancel()
		opts = append(opts, server.WithRedis(rc))
	} else {
		log.Info("REDIS_URL not set: rate limits apply per instance")
	}

	s3Client, err := storage.New(ctx, storage.Options{
		AccessKey:      cfg.S3.AccessKey,
		SecretKey:      cfg.S3.SecretKey,
		ServiceURL:     cfg.S3.ServiceURL,
		Region:         cfg.S3.Region,
		Bucket:         cfg.S3.BucketName,
		ForcePathStyle: cfg.S3.ForcePathStyle,
		VerifySSL:      cfg.S3.VerifySSL,
		CDNBaseURL:     cfg.CDNBaseURL,
	})
	if err != nil {
		log.Error("storage client initialization failed", "error", err)
		os.Exit(1)
	}

	discord := &auth.DiscordClient{
		ClientID:     cfg.DiscordClientID,
		ClientSecret: cfg.DiscordClientSecret,
		RedirectURI:  cfg.DiscordRedirectURI,
	}

	srv := server.New(cfg, pool, s3Client, discord, log, opts...)
	srv.RegisterAll()
	srv.StartBackgroundJobs()

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	var exitCode atomic.Int32
	go func() {
		log.Info("server listening", "addr", *addr, "basePath", cfg.APIBasePath)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			exitCode.Store(1)
			stop()
		}
	}()

	<-ctx.Done()
	stop()
	log.Info("shutting down")
	srv.BeginDrain()
	if d := cfg.Cluster.DrainDelay; d > 0 {
		time.Sleep(d)
	}
	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 20*time.Second)
	if err := httpSrv.Shutdown(drainCtx); err != nil {
		log.Warn("http shutdown incomplete, closing remaining connections", "error", err)
		_ = httpSrv.Close()
	}
	cancelDrain()
	bgCtx, cancelBg := context.WithTimeout(context.Background(), 15*time.Second)
	if err := srv.Shutdown(bgCtx); err != nil {
		log.Warn("background shutdown incomplete", "error", err)
	}
	cancelBg()
	closed := make(chan struct{})
	go func() {
		lockPool.Close()
		pool.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		log.Warn("database connections still in use at exit")
	}
	log.Info("stopped")
	os.Exit(int(exitCode.Load()))
}

func parseLogLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
