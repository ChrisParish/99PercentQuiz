package main

import (
	"context"
	"errors"
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

func main() {
	if err := run(); err != nil {
		log.Fatalf("startup failed: %v", err)
	}
}

func run() error {
	port := env("PORT", "8080")
	title := env("QUIZ_TITLE", "The 99% Quiz")
	tz := env("QUIZ_TZ", "Europe/London")
	stateDir := env("STATE_DIR", "/data/state")

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

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	schedulePath := filepath.Join(stateDir, "schedule.json")

	server, err := NewServer(NewRotation(questions, seed, schedulePath), len(questions), title, loc)
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
		log.Printf("%s serving %d questions on port %s (%s)", title, len(questions), port, tz)
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
