package api

import (
	"fmt"
	"github.com/kubewise/kubewise/internal/api/handler"
	"github.com/kubewise/kubewise/internal/api/router"
	"github.com/labstack/echo/v5"
)

type Server struct {
	echo    *echo.Echo
	handler *handler.Handler
}

func NewServer() *Server {
	e := echo.New()
	handler, err := handler.NewHandler()
	if err != nil {
		panic(fmt.Errorf("初始化API Handler失败: %w", err))
	}
	router.InitRouter(e, handler)
	return &Server{echo: e, handler: handler}
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}
