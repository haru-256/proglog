package main

import (
	"log/slog"
	"os"

	"github.com/haru-256/proglog/internal/server"
)

func main() {
	srv := server.NewHTTPServer(":8080")
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}

func init() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
}
