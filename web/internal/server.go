package internal

import (
	webapi "github.com/akzj/go-lua/web/api"
)

// HTTPServer is an alias for the API type.
type HTTPServer = webapi.HTTPServer

// NewHTTPServer creates a new HTTPServer.
func NewHTTPServer() webapi.HTTPServer {
	return webapi.NewHTTPServer()
}
