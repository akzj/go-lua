// Package table implements Lua tables (array + hash + metatable).
//
// This package imports internal to trigger init() order.
// Init order: mem/internal → string/internal → table/internal
package table

import (
	_ "github.com/akzj/go-lua/table/internal"
)

// Lib is exported to force the import of table/internal
var Lib struct{}

