package logger

import (
	"log/slog"
	"os"
)

// New returns a slog.Logger configured for the given environment.
// - "prod"/"staging": JSON output, so log aggregators / grep-by-field work.
// - anything else (dev, empty, etc.): human-readable text output.
func New(environment string) *slog.Logger {
	level := slog.LevelInfo

	opts := &slog.HandlerOptions{
		Level: level,
		// AddSource pins the file:line each log line came from — handy
		// once you're staring at remote logs instead of your editor.
		AddSource: true,
	}

	var handler slog.Handler
	switch environment {
	case "prod", "staging":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}
