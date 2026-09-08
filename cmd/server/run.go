package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"

	"crowns/internal/httpapi"
	"crowns/internal/matches"
	"crowns/web"
)

type shutdownTimeouts struct {
	drain time.Duration
	save  time.Duration
}

func run(ctx context.Context, addr, dbPath string) error {
	service, err := matches.OpenService(dbPath)
	if err != nil {
		return fmt.Errorf("open session database: %w", err)
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.Join(fmt.Errorf("listen: %w", err), service.Close())
	}
	return serve(ctx, listener, service, web.Assets(), shutdownTimeouts{5 * time.Second, matches.ShutdownSaveTimeout})
}

// serve owns the listener, scheduler and session store, including cleanup when
// serving fails before a signal arrives. Each shutdown phase has a separate
// context so canceling requests cannot cancel the final checkpoint.
func serve(ctx context.Context, listener net.Listener, service *matches.Service, assets fs.FS, timeouts shutdownTimeouts) error {
	simulationCtx, stopSimulation := context.WithCancel(ctx)
	defer stopSimulation()
	requestsCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	server := &http.Server{
		Handler:           httpapi.New(service, assets),
		BaseContext:       func(net.Listener) context.Context { return requestsCtx },
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	simulationDone := make(chan struct{})
	go func() { defer close(simulationDone); service.Run(simulationCtx) }()
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	slog.Info("AI of Empires is ready", "url", "http://"+listener.Addr().String())

	var result error
	var serveErr error
	var serveFinished bool
	select {
	case <-ctx.Done():
	case serveErr = <-served:
		serveFinished = true
	}
	slog.Info("shutdown started; freezing games and closing live streams")
	stopSimulation()
	service.BeginShutdown()

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), timeouts.drain)
	drainErr := server.Shutdown(drainCtx)
	cancelDrain()
	if drainErr != nil {
		slog.Warn("HTTP drain failed; closing remaining connections", "error", drainErr)
		result = errors.Join(result, fmt.Errorf("drain HTTP requests: %w", drainErr))
		cancelRequests()
		if err := server.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("force close HTTP connections: %w", err))
		}
	}
	cancelRequests()
	if !serveFinished {
		serveErr = <-served
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		result = errors.Join(result, fmt.Errorf("serve HTTP: %w", serveErr))
	}
	<-simulationDone

	slog.Info("checkpointing sessions before closing SQLite")
	saveCtx, cancelSave := context.WithTimeout(context.Background(), timeouts.save)
	saveErr := service.CloseContext(saveCtx)
	cancelSave()
	if saveErr != nil {
		result = errors.Join(result, fmt.Errorf("save sessions on shutdown: %w", saveErr))
	} else {
		slog.Info("sessions checkpointed and database closed")
	}
	return result
}
