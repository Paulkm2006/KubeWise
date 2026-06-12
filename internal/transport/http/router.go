package transporthttp

import (
	"net/http"

	activityfeedhttp "github.com/kubewise/kubewise/internal/activityfeed/interface/http"
	conversationhttp "github.com/kubewise/kubewise/internal/conversation/interface/http"
	diaghttp "github.com/kubewise/kubewise/internal/diagnosis/interface/http"
	obshttp "github.com/kubewise/kubewise/internal/observability/interface/http"
	"github.com/kubewise/kubewise/internal/transport/http/middlewares"
	"github.com/kubewise/kubewise/internal/transport/httputil"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func MountRoutes(
	e *echo.Echo,
	conversation *conversationhttp.Handler,
	diagnosis *diaghttp.Handler,
	observability *obshttp.Handler,
	activity *activityfeedhttp.Handler,
) {
	e.Use(middlewares.ZapLogger())
	e.Use(middleware.Recover())
	e.Use(middlewares.CORSMiddleware())

	e.GET("/health", func(c *echo.Context) error {
		log.Ctx(c.Request().Context()).Debug("health check")
		return c.JSON(http.StatusOK, httputil.HealthResponse{Status: "ok", Version: "dev"})
	})
	e.GET("/", func(c *echo.Context) error {
		return c.String(200, "KubeWise — server running")
	})

	v1 := e.Group("/api/v1")

	v1.POST("/chats", conversation.ChatSync)
	v1.GET("/chats/stream", conversation.ChatStream)
	v1.POST("/chats/interactions", conversation.ChatInteraction)

	v1.GET("/sessions", conversation.ListSessions)
	v1.POST("/sessions", conversation.CreateSession)
	v1.GET("/sessions/:id", conversation.GetSession)
	v1.DELETE("/sessions/:id", conversation.DeleteSession)

	v1.GET("/clusters", observability.ListClusters)
	v1.GET("/clusters/:name/issues", observability.ListIssues)
	v1.GET("/clusters/:name/events", observability.ListEvents)

	v1.POST("/diagnoses", diagnosis.Start)
	v1.GET("/diagnoses", diagnosis.List)
	v1.GET("/diagnoses/latest", diagnosis.Latest)
	v1.GET("/diagnoses/:id", diagnosis.Get)
	v1.POST("/diagnoses/:id/cancel", diagnosis.Cancel)
	v1.GET("/diagnoses/:id/events", diagnosis.StreamEvents)

	v1.GET("/activities", activity.List)
}
