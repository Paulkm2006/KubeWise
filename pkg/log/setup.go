// Package log configures process-wide zap logging for CLI/TUI.
package log

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Options controls logger construction.
type Options struct {
	Verbose          bool
	Level            string // debug / info / warn / error
	File             string // stderr, -, a path, or empty
	LevelFlagChanged bool   // user passed --log-level
	FileFlagChanged  bool   // user passed --log-file
}

// New returns a logger. When Verbose is false, returns a no-op logger.
func New(opts Options) (*zap.Logger, error) {
	if !opts.Verbose {
		return zap.NewNop(), nil
	}

	levelStr := strings.TrimSpace(opts.Level)
	if levelStr == "" {
		levelStr = "debug"
	}
	// -v is for troubleshooting: default to debug unless the user set --log-level.
	if !opts.LevelFlagChanged && levelStr == "info" {
		levelStr = "debug"
	}

	level, err := parseLevel(levelStr)
	if err != nil {
		return nil, err
	}

	atomicLevel := zap.NewAtomicLevelAt(level)
	sink, sinkDesc, err := resolveSink(opts.File, opts.FileFlagChanged)
	if err != nil {
		return nil, err
	}

	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	if sink == os.Stderr {
		enc = zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	}

	core := zapcore.NewCore(enc, zapcore.AddSync(sink), atomicLevel)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	logger.Info("verbose logging enabled",
		zap.String("level", levelStr),
		zap.String("sink", sinkDesc),
	)
	return logger, nil
}

// resolveSink picks the log destination. File paths write only to disk so TUI
// output is not polluted; stderr is used only when explicitly requested.
func resolveSink(file string, fileFlagChanged bool) (sink *os.File, desc string, err error) {
	path := strings.TrimSpace(file)

	// -v without --log-file: default file, not terminal.
	if path == "" || (path == "stderr" && !fileFlagChanged) {
		path = "./kubewise.log"
	}

	if path == "stderr" || path == "-" {
		return os.Stderr, "stderr", nil
	}

	f, err := openLogFile(path)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

func parseLevel(s string) (zapcore.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("unknown log level %q (use debug, info, warn, or error)", s)
	}
}

func openLogFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}
	return f, nil
}
