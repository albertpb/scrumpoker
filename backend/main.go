package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"scrumpoker/backend/application"
	"scrumpoker/backend/infrastructure/httpapi"
	"scrumpoker/backend/infrastructure/sqlite"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	databasePath := envOrDefault("DATABASE_PATH", "poker.db")
	repository, err := sqlite.Open(databasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer repository.Close()

	service := application.NewService(repository)
	handler := httpapi.NewHandler(service, logger, os.Getenv("FRONTEND_DIR"))

	server := &http.Server{
		Addr:              envOrDefault("ADDR", ":8080"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("server listening", "address", server.Addr, "database", databasePath)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
