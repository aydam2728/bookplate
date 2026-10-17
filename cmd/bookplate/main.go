// Command bookplate runs the BookPlate server.
//
// It resolves the configuration, opens and migrates the database, assembles
// the routes and runs until the process is asked to stop. Everything else
// lives behind a package so it can be tested without starting a process.
//
// The "healthcheck" subcommand probes a running instance. The container image
// uses it so that it needs no shell, curl or wget of its own.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/aydam2728/bookplate/internal/auth"
	"github.com/aydam2728/bookplate/internal/catalog"
	"github.com/aydam2728/bookplate/internal/covers"
	"github.com/aydam2728/bookplate/internal/metadata"
	"github.com/aydam2728/bookplate/internal/places"
	"github.com/aydam2728/bookplate/internal/platform/config"
	"github.com/aydam2728/bookplate/internal/platform/database"
	"github.com/aydam2728/bookplate/internal/platform/httpserver"
	"github.com/aydam2728/bookplate/internal/web"
)

// version is set at build time with -ldflags. It identifies this instance
// to the metadata providers.
var version = "dev"

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

	db, err := database.Open(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := prepareSchema(ctx, db, cfg.DataDir, logger); err != nil {
		return err
	}

	accounts := auth.NewHandlers(auth.NewStore(db), logger)

	// One fetcher per purpose, each allowed only the hosts it needs. A
	// catalogue lookup has no business reaching an image host, and neither
	// has any business reaching the other catalogue.
	agent := userAgent(cfg.Contact)
	catalogueFetcher := metadata.NewFetcher([]string{metadata.OpenLibraryHost}, agent)
	imageFetcher := metadata.NewFetcher(
		[]string{covers.CoversHost, metadata.GoogleThumbnailHost}, agent)
	bnfFetcher := metadata.NewFetcher([]string{metadata.BnFHost}, agent)
	googleFetcher := metadata.NewFetcher([]string{metadata.GoogleBooksHost}, agent)

	coverStore := covers.NewStore(db, cfg.DataDir)
	coverService := covers.NewService(imageFetcher, coverStore, logger)
	coverRoutes := covers.NewHandlers(coverStore, logger)

	books := catalog.NewHandlers(catalog.NewStore(db), coverService, logger)
	shelves := places.NewHandlers(places.NewStore(db), logger)

	// Open Library is the only provider enabled without an operator opting
	// in: its data is in the public domain and it needs no key.
	// Order matters: open data first, then the French legal deposit, then
	// the one whose terms are most restrictive.
	providers := metadata.Chain{
		metadata.NewOpenLibrary(catalogueFetcher),
		metadata.NewBnF(bnfFetcher),
	}
	// Google knows a good deal of recent publishing that Open Library does
	// not, but only with the operator's own key. Without one it stays off.
	if google := metadata.NewGoogleBooks(googleFetcher, cfg.GoogleBooksKey); google != nil {
		providers = append(providers, google)
		logger.Info("google books enabled")
	}
	lookups := metadata.NewHandlers(providers, catalog.NormaliseISBN, logger)

	srv := httpserver.New(cfg.Addr, logger,
		func(ctx context.Context) error { return database.Ready(ctx, db) },
		accounts.Register,
		func(mux *http.ServeMux) { books.Register(mux, accounts.Protect) },
		func(mux *http.ServeMux) { lookups.Register(mux, accounts.Protect) },
		func(mux *http.ServeMux) { coverRoutes.Register(mux, accounts.Protect) },
		func(mux *http.ServeMux) { shelves.Register(mux, accounts.Protect) },
		web.Register,
	)
	// Security headers wrap everything, including the operational endpoints.
	srv.Handler = web.Harden(srv.Handler)

	if err := httpserver.Run(ctx, srv, logger); err != nil {
		return fmt.Errorf("serving: %w", err)
	}
	logger.Info("stopped")
	return nil
}

// prepareSchema backs up an existing database before changing its shape, then
// migrates. A refusal here stops the process on purpose: starting on a schema
// the binary does not understand is how data gets corrupted.
func prepareSchema(ctx context.Context, db *sql.DB, dir string, logger *slog.Logger) error {
	existing, err := database.HasSchema(ctx, db)
	if err != nil {
		return err
	}
	if existing {
		path, err := database.Backup(ctx, db, dir)
		if err != nil {
			return fmt.Errorf("refusing to migrate without a backup: %w", err)
		}
		logger.Info("backed up before migrating", "path", path)
	}

	applied, err := database.Migrate(ctx, db, database.Migrations())
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		logger.Info("schema migrated", "applied", applied)
	}
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

// userAgent identifies this instance to metadata providers. Open Library
// raises its rate limit for callers who say who they are, and an operator who
// can be warned is one who does not get blocked outright.
func userAgent(contact string) string {
	agent := "BookPlate/" + version + " (+https://github.com/aydam2728/bookplate)"
	if contact != "" {
		agent = "BookPlate/" + version + " (" + contact + ")"
	}
	return agent
}
