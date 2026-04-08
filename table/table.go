// Package table implements Lua tables (array + hash + metatable).
//
// This package imports internal to trigger init() order.
// Init order: mem/internal → string/internal → table/internal
package table

import (
	"github.com/akzj/go-lua/table/api"
	"github.com/akzj/go-lua/table/internal"
	types "github.com/akzj/go-lua/types/api"
)

// Lib is exported to force the import of table/internal
var Lib struct{}

// NewTable returns a fresh table instance from the internal package.
// This factory avoids the singleton issue in table/api.NewTable for use cases
// that need distinct table instances.
func NewTable() api.TableInterface {
	return internal.NewTable()
}

// WrapRawTable wraps a types.Table (internal *Table) back into a TableInterface.
// Used by getmetatable to convert the raw metatable back to a usable interface.
func WrapRawTable(t types.Table) api.TableInterface {
	return internal.WrapRawTable(t)
}

