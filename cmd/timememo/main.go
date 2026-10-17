package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"timememo/internal/server"
	"timememo/internal/store"
	"timememo/web"
)

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	s, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		cancel()
		return err
	}
	defer s.DB.Close()
	if err = s.Migrate(ctx); err != nil {
		cancel()
		return err
	}
	cancel()
	handler, err := server.New(s, server.Config{UploadDir: os.Getenv("UPLOAD_DIR"), SecureCookie: os.Getenv("COOKIE_SECURE") == "true", Assets: web.Assets()})
	if err != nil {
		return err
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	httpServer := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopped := make(chan struct{})
	go func() {
		<-stopCtx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Warn("graceful shutdown timed out", "error", err)
			_ = httpServer.Close()
		}
		close(stopped)
	}()
	slog.Info("时光记 TimeMemo started", "url", "http://"+addr)
	if err = httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-stopped
	return nil
}
func main() {
	if err := run(); err != nil {
		slog.Error("TimeMemo stopped", "error", err)
		os.Exit(1)
	}
}
