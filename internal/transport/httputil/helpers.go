package httputil

import (
	"context"

	"github.com/kubewise/kubewise/internal/transport/http/ssestream"
	"github.com/labstack/echo/v5"
)

func ContextWithCancel(c *echo.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(c.Request().Context())
}

func NewSSE(c *echo.Context) (*ssestream.SSEWriter, error) {
	return ssestream.NewSSEWriter(c.Response())
}
