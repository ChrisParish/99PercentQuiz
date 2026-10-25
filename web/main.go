package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	// Embeds the tz database in the binary. The distroless runtime image has no
	// tzdata, so without this Europe/London silently falls back to UTC.
	_ "time/tzdata"
)

// version identifies the running build. The Dockerfile bakes the image tag in
// via -ldflags; QUIZ_VERSION overrides it when nothing was baked in.
var version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatalf("startup failed: %v", err)
	}
}

func run() error {
	port := env("PORT", "8080")
	title := env("QUIZ_TITLE", "The 99% Quiz")
	tz := env("QUIZ_TZ", "Europe/London")
	release := env("QUIZ_VERSION", version)

	// An explicitly empty STATE_DIR opts out of persistence; unset means the
	// usual mount point, so a missing volume is an error rather than a silent
	// downgrade to losing every swap on restart.
	stateDir, ok := os.LookupEnv("STATE_DIR")
	if !ok {
		stateDir = "/data/state"
	}

	seed, err := strconv.ParseUint(env("QUIZ_SEED", "20260105"), 10, 64)
	if err != nil {
		return errors.New("QUIZ_SEED must be a positive integer")
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return err
	}

	questions, err := loadQuestions()
	if err != nil {
		return err
	}

	var schedulePath string
	if stateDir != "" {
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			return fmt.Errorf("creating state directory %s: %w", stateDir, err)
		}
		if err := ensureWritable(stateDir); err != nil {
			return fmt.Errorf("state directory %s is not writable by uid %d, so swaps could not be saved: %w"+
				" - chown the volume to 65532:65532, or set STATE_DIR= to run without saving swaps",
				stateDir, os.Getuid(), err)
		}
		schedulePath = filepath.Join(stateDir, "schedule.json")
	} else {
		log.Print("STATE_DIR is empty: swaps will be lost when this container stops")
	}

	server, err := NewServer(NewRotation(questions, seed, schedulePath), len(questions), title, release, loc)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	errs := make(chan error, 1)
	go func() {
		log.Printf("%s %s serving %d questions on port %s (%s)", title, release, len(questions), port, tz)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-shutdown:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		log.Print("shutting down")
		return srv.Shutdown(ctx)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ensureWritable proves the state directory can actually be written, so a
// misconfigured volume fails the deploy instead of surfacing when a presenter
// first presses Swap.
func ensureWritable(dir string) error {
	probe := filepath.Join(dir, ".write-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Remove(probe)
}
