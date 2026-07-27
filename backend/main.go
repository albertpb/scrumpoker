package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"scrumpoker/backend/application"
	"scrumpoker/backend/infrastructure/config"
	"scrumpoker/backend/infrastructure/httpapi"
	"scrumpoker/backend/infrastructure/postgres"
	"scrumpoker/backend/infrastructure/sqlite"
)

type repository interface {
	application.RoomRepository
	Close() error
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := config.LoadEnv(".env"); err != nil {
		logger.Error("load .env", "error", err)
		os.Exit(1)
	}
	databaseDriver := strings.ToLower(strings.TrimSpace(envOrDefault("DATABASE_DRIVER", "sqlite")))
	repository, err := openRepository(databaseDriver)
	if err != nil {
		logger.Error("open database", "driver", databaseDriver, "error", err)
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

	logger.Info("server listening", "address", server.Addr, "database_driver", databaseDriver)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func openRepository(driver string) (repository, error) {
	switch driver {
	case "sqlite":
		return sqlite.Open(envOrDefault("DATABASE_PATH", "poker.db"))
	case "postgres", "postgresql":
		connectionURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
		if connectionURL == "" {
			return nil, errors.New("DATABASE_URL is required when DATABASE_DRIVER is postgres")
		}
		return postgres.Open(connectionURL)
	default:
		return nil, fmt.Errorf("unsupported DATABASE_DRIVER %q; use sqlite or postgres", driver)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
