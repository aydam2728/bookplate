// Command bookplate runs the BookPlate server.
//
// It does three things and nothing else: resolve the configuration, build the
// server, and run it until the process is asked to stop. Everything else lives
// behind a package, so that it can be tested without starting a process.
//
// The "healthcheck" subcommand probes a running instance. The container image
// uses it so that it needs no shell, curl or wget of its own.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aydam2728/bookplate/internal/platform/config"
	"github.com/aydam2728/bookplate/internal/platform/httpserver"
)

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		// The logger may not exist yet when configuration fails, so this one
		// message goes straight to stderr.
		fmt.Fprintln(os.Stderr, "bookplate:", err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	switch {
	case len(args) == 0:
		return serve()
	case args[0] == "healthcheck":
		return healthcheck()
	default:
		return fmt.Errorf("unknown command %q, expected none or \"healthcheck\"", args[0])
	}
}

func serve() error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return fmt.Errorf("configuration:\n%w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	logger.Info("starting", "data_dir", cfg.DataDir, "log_level", cfg.LogLevel.String())

	// Interrupt and terminate are both expected: the first from a terminal,
	// the second from the container runtime.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Nothing to wait for yet. The database readiness probe lands with the
	// database.
	srv := httpserver.New(cfg.Addr, logger, nil)

	if err := httpserver.Run(ctx, srv, logger); err != nil {
		return fmt.Errorf("serving: %w", err)
	}
	logger.Info("stopped")
	return nil
}

// healthcheck probes the instance this process would itself serve. It writes
// nothing on success: the container runtime reads the exit status.
func healthcheck() error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return fmt.Errorf("configuration:\n%w", err)
	}
	return httpserver.Probe(context.Background(), cfg.Addr)
}
