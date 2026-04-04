package internal

import (
	webapi "github.com/akzj/go-lua/web/api"
)

// RequestContext is an alias for the API type.
type RequestContext = webapi.RequestContext

// NewRequestContext creates a new RequestContext.
func NewRequestContext() *RequestContext {
	return webapi.NewRequestContext()
}

// Handler is an alias for the API type.
type Handler = webapi.Handler

// HandlerFunc is an alias for the API type.
type HandlerFunc = webapi.HandlerFunc
