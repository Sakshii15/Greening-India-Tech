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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"github.com/taskflow/backend/internal/db"
	"github.com/taskflow/backend/internal/handler"
	mw "github.com/taskflow/backend/internal/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_DB"),
	)

	conn, err := connectWithRetry(dsn, 10)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := db.RunMigrations(dsn); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied successfully")

	if err := db.RunSeed(conn); err != nil {
		slog.Error("failed to run seed", "error", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Error("JWT_SECRET is required")
		os.Exit(1)
	}

	h := handler.New(conn, jwtSecret)
	authMw := mw.Auth(jwtSecret)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(mw.JSONContentType)

	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)

	r.Group(func(r chi.Router) {
		r.Use(authMw)

		r.Get("/projects", h.ListProjects)
		r.Post("/projects", h.CreateProject)
		r.Get("/projects/{id}", h.GetProject)
		r.Patch("/projects/{id}", h.UpdateProject)
		r.Delete("/projects/{id}", h.DeleteProject)
		r.Get("/projects/{id}/tasks", h.ListTasks)
		r.Post("/projects/{id}/tasks", h.CreateTask)
		r.Get("/projects/{id}/stats", h.ProjectStats)

		r.Patch("/tasks/{id}", h.UpdateTask)
		r.Delete("/tasks/{id}", h.DeleteTask)
	})

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	slog.Info("server stopped")
}

func connectWithRetry(dsn string, maxRetries int) (*sql.DB, error) {
	var conn *sql.DB
	var err error
	for i := 0; i < maxRetries; i++ {
		conn, err = sql.Open("postgres", dsn)
		if err == nil {
			err = conn.Ping()
		}
		if err == nil {
			return conn, nil
		}
		slog.Info("waiting for database...", "attempt", i+1)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("could not connect after %d attempts: %w", maxRetries, err)
}
