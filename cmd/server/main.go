package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9090", "HTTP listen address")
	dbPath := flag.String("db", "data/ai-of-empires.sqlite", "Host catalog path; game files live in <path>.games/ (:memory: for disposable games)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, *addr, *dbPath)
	// Keep signal handling installed through the final save. A second signal
	// must not accidentally abort checkpointing when the HTTP listener closes.
	stop()
	if err != nil {
		slog.Error("server stopped with errors", "error", err)
		os.Exit(1)
	}
}
