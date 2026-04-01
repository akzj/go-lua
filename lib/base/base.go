// Package base provides the Lua base library.
//
// This package exports the base library for use by the VM's OpenLibs() function.
//
// Usage:
//
//	import "github.com/akzj/go-lua/lib/base"
//	// In VM initialization:
//	vm.OpenLibs(base.Lib)
package base

import (
	baseapi "github.com/akzj/go-lua/lib/base/api"
	"github.com/akzj/go-lua/lib/base/internal"
)

// Lib is the base library instance.
// Use this with vm.OpenLibs().
var Lib baseapi.BaseLib = internal.NewBaseLib()
