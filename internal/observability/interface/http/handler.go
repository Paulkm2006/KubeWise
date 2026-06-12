package httpapi

import (
	"errors"
	"net/http"

	obsapp "github.com/kubewise/kubewise/internal/observability/application"
	obscluster "github.com/kubewise/kubewise/internal/observability/infrastructure/cluster"
	httputil "github.com/kubewise/kubewise/internal/transport/httputil"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type Handler struct {
	Service *obsapp.Service
}

func (h *Handler) ListClusters(c *echo.Context) error {
	ctx := c.Request().Context()
	clusters := h.Service.ListClusters(ctx)
	log.Ctx(ctx).Info("listed clusters", zap.Int("count", len(clusters)))
	return c.JSON(http.StatusOK, clusters)
}

func (h *Handler) ListIssues(c *echo.Context) error {
	ctx := c.Request().Context()
	name := c.Param("name")
	if name == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "cluster name required"})
	}

	issues, err := h.Service.ListIssues(ctx, name)
	if err != nil {
		return mapClusterError(c, err)
	}

	log.Ctx(ctx).Info("listed cluster issues", zap.String("cluster", name), zap.Int("count", len(issues)))
	return c.JSON(http.StatusOK, issues)
}

func (h *Handler) ListEvents(c *echo.Context) error {
	ctx := c.Request().Context()
	name := c.Param("name")

	events, err := h.Service.ListEvents(ctx, name)
	if err != nil {
		return mapClusterError(c, err)
	}

	log.Ctx(ctx).Info("listed cluster events", zap.String("cluster", name), zap.Int("count", len(events)))
	return c.JSON(http.StatusOK, events)
}

func mapClusterError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, obscluster.ErrNameRequired):
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: err.Error()})
	case errors.Is(err, obscluster.ErrNotFound):
		return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: err.Error()})
	case errors.Is(err, obscluster.ErrOffline):
		return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: err.Error()})
	case errors.Is(err, obscluster.ErrUnavailable):
		return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "cluster gateway unavailable"})
	default:
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: err.Error()})
	}
}
