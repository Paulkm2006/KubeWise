package api

import (
	"os"
	"path/filepath"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

type Server struct {
	echo    *echo.Echo
	handler *Handler
}

var indexHTML string

// ServeIndex loads the frontend HTML once at startup.
func ServeIndex(siteDir string) {
	data, err := os.ReadFile(filepath.Join(siteDir, "index.html"))
	if err != nil {
		return
	}
	indexHTML = string(data)
}

func NewServer(handler *Handler) *Server {
	e := echo.New()

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(CORSMiddleware())

	e.GET("/health", handler.Health)

	v1 := e.Group("/api/v1")
	v1.POST("/chat", handler.ChatSync)
	v1.GET("/chat/stream", handler.ChatStream)
	v1.POST("/chat/interaction", handler.ChatInteraction)

	v1.GET("/cluster/status", handler.ClusterStatus)

	v1.GET("/sessions", handler.ListSessions)
	v1.POST("/sessions", handler.CreateSession)
	v1.GET("/sessions/:id", handler.GetSession)
	v1.DELETE("/sessions/:id", handler.DeleteSession)

	// SPA: serve index.html for root only (NOT a catch-all static handler)
	e.GET("/", func(c *echo.Context) error {
		if indexHTML != "" {
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			return c.HTML(200, indexHTML)
		}
		return c.String(200, "KubeWise — server running")
	})

	return &Server{echo: e, handler: handler}
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}
