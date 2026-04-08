// Package table implements Lua tables (array + hash + metatable).
//
// This package imports internal to trigger init() order.
// Init order: mem/internal → string/internal → table/internal
package table

import (
	"github.com/akzj/go-lua/table/api"
	"github.com/akzj/go-lua/table/internal"
)

// Lib is exported to force the import of table/internal
var Lib struct{}

// NewTable returns a fresh table instance from the internal package.
// This factory avoids the singleton issue in table/api.NewTable for use cases
// that need distinct table instances.
func NewTable() api.TableInterface {
	return internal.NewTable()
}

