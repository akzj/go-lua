// Package string provides the Lua string library.
//
// This package exports the string library for use by the VM's OpenLibs() function.
//
// Usage:
//
//	import "github.com/akzj/go-lua/lib/string"
//	// In VM initialization:
//	vm.OpenLibs(string.Lib)
package string

import (
	stringapi "github.com/akzj/go-lua/lib/string/api"
	"github.com/akzj/go-lua/lib/string/internal"
)

// Lib is the string library instance.
// Use this with vm.OpenLibs().
var Lib stringapi.StringLib = internal.NewStringLib()
