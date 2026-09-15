package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"relay-server/internal/api"
	"relay-server/internal/store"
	"relay-server/migrations"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:18443"
	}
	ctx := context.Background()
	var s *api.Server
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pg, err := store.NewPostgres(ctx, dsn)
		if err != nil {
			log.Fatal(err)
		}
		defer pg.Close()
		if err := runMigrations(ctx, pg); err != nil {
			// A partially migrated database must never start serving traffic. Apply
			// is transactional and stops at the first failed migration.
			log.Fatal(err)
		}
		s = api.NewServer(pg)
		configureFCM(s)
		log.Println("using PostgreSQL store")
	} else {
		log.Println("DATABASE_URL unset; using in-memory store")
		s = api.NewServer()
		configureFCM(s)
	}
	// Server is the product concept; the legacy relay-server entry path remains compatible.
	httpServer := &http.Server{Addr: addr, Handler: s.Handler()}
	serverErr := make(chan error, 1)
	go func() { serverErr <- httpServer.ListenAndServe() }()
	log.Printf("server listening on %s", addr)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	case <-stop:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
		if err := s.Close(); err != nil {
			log.Printf("realtime shutdown: %v", err)
		}
	}
}

// configureFCM is intentionally a deployment hook. Production must inject both
// an OAuth access-token source and a hash-to-token resolver from Secret Manager
// or workload identity; this binary never reads service-account JSON or raw token
// values from environment variables.
func configureFCM(s *api.Server) {
	if strings.TrimSpace(os.Getenv("FCM_ENABLED")) == "" || !strings.EqualFold(strings.TrimSpace(os.Getenv("FCM_ENABLED")), "true") {
		return
	}
	// This remains fail-closed until the deployment supplies concrete
	// SecretManager-backed implementations for both interfaces. Do not create a
	// dispatcher with placeholders: that would advertise delivery without a
	// resolver capable of retrieving the raw token at send time.
	log.Println("FCM_ENABLED=true but no secret-backed access-token source/resolver is wired; push dispatch remains disabled")
	_ = s
}

// runMigrations applies the embedded migration set before the HTTP listener is
// opened. MIGRATIONS_MODE accepts apply (default), dry-run, and off. The
// explicit off mode is intended only for operators that run migrations as a
// separate deployment step; invalid values fail closed.
func runMigrations(ctx context.Context, pg *store.PostgresStore) error {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("MIGRATIONS_MODE")))
	if mode == "" {
		mode = "apply"
	}
	switch mode {
	case "off", "disabled", "false", "0":
		log.Println("database migrations disabled by MIGRATIONS_MODE")
		return nil
	case "apply", "auto", "true", "1":
	case "dry-run", "dryrun", "plan":
	default:
		return fmt.Errorf("invalid MIGRATIONS_MODE %q (want apply, dry-run, or off)", mode)
	}
	runner, err := migrations.New()
	if err != nil {
		return fmt.Errorf("load database migrations: %w", err)
	}
	if strings.HasPrefix(mode, "dry") || mode == "plan" {
		plan, err := runner.DryRun(ctx, pg.Pool)
		if err != nil {
			return fmt.Errorf("inspect database migrations: %w", err)
		}
		log.Printf("database migration dry-run: %d applied, %d pending", len(plan.Applied), len(plan.Pending))
		return nil
	}
	result, err := runner.Apply(ctx, pg.Pool)
	if err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}
	log.Printf("database migrations complete: applied %d", len(result.Applied))
	return nil
}
