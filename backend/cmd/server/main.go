package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
	"github.com/waifu-im/waifu-api/backend/internal/db"
	"github.com/waifu-im/waifu-api/backend/internal/server"
	"github.com/waifu-im/waifu-api/backend/internal/storage"
)

func main() {
	envPath := flag.String("env", "", "path to the .env file (auto-detected when empty)")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	cfg, err := config.Load(*envPath)
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	log.Info("configuration loaded", "env", *envPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DBURL())
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	log.Info("database connected", "host", cfg.DBHost, "db", cfg.DBName)

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

	srv := server.New(cfg, pool, s3Client, discord, log)
	srv.RegisterAuthRoutes()
	srv.RegisterHealthRoutes()
	srv.RegisterOpenAPIRoutes()
	srv.RegisterImageRoutes()
	srv.RegisterTagRoutes()
	srv.RegisterArtistRoutes()
	srv.RegisterUserRoutes()
	srv.RegisterAlbumRoutes()
	srv.RegisterReviewRoutes()
	srv.RegisterReportRoutes()
	srv.RegisterBannerRoutes()
	srv.RegisterStatsRoutes()
	srv.RegisterStorageDiffRoutes()

	srv.StartBackgroundJobs()

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info("server listening", "addr", *addr, "basePath", cfg.APIBasePath)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}
