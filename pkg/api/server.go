package api

import (
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

type Server struct {
	echo    *echo.Echo
	handler *Handler
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
	v1.POST("/chat/confirm", handler.ChatConfirm)

	v1.GET("/sessions", handler.ListSessions)
	v1.POST("/sessions", handler.CreateSession)
	v1.GET("/sessions/:id", handler.GetSession)
	v1.DELETE("/sessions/:id", handler.DeleteSession)

	return &Server{echo: e, handler: handler}
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}
