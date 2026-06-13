package log

import (
	"context"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/config"
)

// TraceIDKey is the context key for request trace IDs.
// Exported so middleware and other packages can read/write with the same key.
const TraceIDKey ctxKey = "trace_id"

type ctxKey string

// Ctx returns a logger with the trace_id from ctx attached as a structured field.
// If ctx is nil or has no trace_id, returns the global logger from config.L().
func Ctx(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return config.L()
	}
	if id, ok := ctx.Value(TraceIDKey).(string); ok && id != "" {
		return config.L().With(zap.String("trace_id", id))
	}
	return config.L()
}
