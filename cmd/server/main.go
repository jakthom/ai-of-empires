package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"crowns/internal/httpapi"
	"crowns/internal/matches"
	"crowns/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9090", "HTTP listen address")
	dbPath := flag.String("db", "data/ai-of-empires.sqlite", "SQLite session database path (:memory: for disposable games)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service, err := matches.OpenService(*dbPath)
	if err != nil {
		slog.Error("open session database", "error", err)
		os.Exit(1)
	}
	simulationDone := make(chan struct{})
	go func() { service.Run(ctx); close(simulationDone) }()
	server := &http.Server{Addr: *addr, Handler: httpapi.New(service, web.Assets()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("AI of Empires is ready", "url", "http://"+*addr)
	serveErr := server.ListenAndServe()
	if err := serveErr; err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
	}
	stop()
	<-simulationDone
	<-shutdownDone
	if err := service.Close(); err != nil {
		slog.Error("save sessions on shutdown", "error", err)
		os.Exit(1)
	}
	if serveErr != nil && serveErr != http.ErrServerClosed {
		os.Exit(1)
	}
}
