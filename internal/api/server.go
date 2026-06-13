// Package api is the legacy entrypoint alias for the HTTP transport layer.
package api

import transporthttp "github.com/kubewise/kubewise/internal/transport/http"

type Server = transporthttp.Server

func NewServer() *Server {
	return transporthttp.NewServer()
}
