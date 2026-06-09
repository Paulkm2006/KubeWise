package router

import (
	"github.com/kubewise/kubewise/internal/api/handler"
	"github.com/kubewise/kubewise/internal/api/middlewares"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func InitRouter(e *echo.Echo, h *handler.Handler) {
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middlewares.CORSMiddleware())
	e.GET("/health", h.Health)

	e.GET("/", func(c *echo.Context) error {
		return c.String(200, "KubeWise — server running")
	})

	v1 := e.Group("/api/v1")
	v1.POST("/chat", h.ChatSync)
	v1.GET("/chat/stream", h.ChatStream)
	v1.POST("/chat/interaction", h.ChatInteraction)

	v1.GET("/cluster/status", h.ClusterStatus)

	// Dashboard
	v1.GET("/clusters", h.ListClusters)
	v1.GET("/clusters/:name/issues", h.ListIssues)
	v1.GET("/clusters/:name/events", h.ListClusterEvents)

	// Diagnosis
	v1.POST("/diagnose", h.StartDiagnose)
	v1.GET("/diagnoses", h.ListDiagnoses)
	v1.GET("/diagnoses/:id", h.GetDiagnosis)
	v1.GET("/diagnose/stream", h.StreamDiagnosisEvents)

	// Activities
	v1.GET("/activities", h.ListActivities)

	v1.GET("/sessions", h.ListSessions)
	v1.POST("/sessions", h.CreateSession)
	v1.GET("/sessions/:id", h.GetSession)
	v1.DELETE("/sessions/:id", h.DeleteSession)
}
