// Command server is the main entry point for the proglog HTTP server.
// It starts a server on port 8080 that provides REST API endpoints for log operations.
//
// The server supports:
//   - POST / : Append new records to the log
//   - GET / : Retrieve records from the log by offset
//
// Usage:
//
//	go run cmd/server/main.go
package main

import (
	"log/slog"
	"os"

	"github.com/haru-256/proglog/internal/server"
)

// main starts the HTTP server and handles any startup errors.
// The server listens on port 8080 and will exit with code 1 if startup fails.
func main() {
	srv := server.NewHTTPServer(":8080")
	slog.Info("starting server", "address", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}

// init configures the default logger to output debug-level logs to stdout.
// This setup ensures all server operations are properly logged during development.
func init() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
}
