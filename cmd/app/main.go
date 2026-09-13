// Command app launches the Zealish WhatsApp desktop client.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/zealish/zealish-whatsapp/internal/app"
	"github.com/zealish/zealish-whatsapp/internal/config"
	"github.com/zealish/zealish-whatsapp/internal/storage"
)

func init() {
	// GTK requires all UI work on the thread that ran gtk_init.
	runtime.LockOSThread()
}

func main() {
	os.Exit(run())
}

func run() int {
	paths, err := storage.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "zealish-wa:", err)
		return 1
	}

	logFile, log := newLogger(paths)
	if logFile != nil {
		defer func() { _ = logFile.Close() }()
	}

	cfg, err := config.Load(paths.ConfigFile())
	if err != nil {
		log.Error("loading config failed", "error", err)
		return 1
	}

	application, err := app.New(cfg, paths, log)
	if err != nil {
		log.Error("starting application failed", "error", err)
		return 1
	}

	return application.Run()
}

func newLogger(paths storage.Paths) (*os.File, *slog.Logger) {
	level := slog.LevelInfo
	if os.Getenv("ZEALISH_WA_DEBUG") != "" {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}

	if err := os.MkdirAll(paths.LogDir(), 0o700); err != nil {
		return nil, slog.New(slog.NewTextHandler(os.Stderr, opts))
	}

	f, err := os.OpenFile(filepath.Join(paths.LogDir(), "zealish-wa.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return f, slog.New(slog.NewTextHandler(f, opts))
}
