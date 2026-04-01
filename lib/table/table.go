// Package table provides the Lua table library.
//
// This package exports the table library for use by the VM's OpenLibs() function.
//
// Usage:
//
//	import "github.com/akzj/go-lua/lib/table"
//	// In VM initialization:
//	vm.OpenLibs(table.Lib)
package table

import (
	tableapi "github.com/akzj/go-lua/lib/table/api"
	"github.com/akzj/go-lua/lib/table/internal"
)

// Lib is the table library instance.
// Use this with vm.OpenLibs().
var Lib tableapi.TableLib = internal.NewTableLib()
