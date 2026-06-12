package middlewares

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/utils/log"
)

// ZapLogger returns an Echo middleware that logs every HTTP request via zap.
// Replaces echo's built-in middleware.RequestLogger().
func ZapLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			req := c.Request()

			// 1. Trace ID: prefer X-Request-ID header, else generate UUID
			traceID := req.Header.Get("X-Request-ID")
			if traceID == "" {
				traceID = uuid.NewString()
			}

			// 2. Store Trace ID in request context
			ctx := context.WithValue(req.Context(), log.TraceIDKey, traceID)
			c.SetRequest(req.WithContext(ctx))
			c.Response().Header().Set("X-Request-ID", traceID)

			// 3. Request start (debug)
			log.Ctx(ctx).Debug("request started",
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.String("query", req.URL.RawQuery),
			)

			// 4. Execute handler chain
			err := next(c)

			// 5. Request completed — Echo v5 Response already tracks Status + Size
			latency := time.Since(start)
			resp := c.Response()

			// Echo v5 returns http.ResponseWriter — underlying type is *echo.Response
			var status int
			var size int64
			if er, ok := resp.(*echo.Response); ok {
				status = er.Status
				size = er.Size
			}

			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.Int("status", status),
				zap.Duration("latency", latency),
				zap.Int64("bytes_out", size),
			}

			// 6. Choose log level by status + latency
			logFn := logFuncFor(status, latency)
			if err != nil {
				fields = append(fields, zap.Error(err))
			}
			logFn(ctx, "request completed", fields...)

			return err
		}
	}
}

func logFuncFor(status int, latency time.Duration) func(ctx context.Context, msg string, fields ...zap.Field) {
	if latency > 1*time.Second {
		return func(ctx context.Context, msg string, fields ...zap.Field) {
			log.Ctx(ctx).Warn(msg, append(fields, zap.Bool("slow", true))...)
		}
	}
	switch {
	case status >= 500:
		return func(ctx context.Context, msg string, fields ...zap.Field) {
			log.Ctx(ctx).Error(msg, fields...)
		}
	case status >= 400:
		return func(ctx context.Context, msg string, fields ...zap.Field) {
			log.Ctx(ctx).Warn(msg, fields...)
		}
	default:
		return func(ctx context.Context, msg string, fields ...zap.Field) {
			log.Ctx(ctx).Info(msg, fields...)
		}
	}
}
