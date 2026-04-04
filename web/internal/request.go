package internal

import (
	webapi "github.com/akzj/go-lua/web/api"
)

// Re-export types from webapi for convenience.
type (
	Response       = webapi.Response
	ResponseHeader = webapi.ResponseHeader
	Request        = webapi.Request
)
