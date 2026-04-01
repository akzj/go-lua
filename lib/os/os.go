// Package os provides the Lua OS library.
//
// This package exports the OS library for use by the VM's OpenLibs() function.
//
// Usage:
//
//	import "github.com/akzj/go-lua/lib/os"
//	// In VM initialization:
//	vm.OpenLibs(os.Lib)
package os

import (
	osapi "github.com/akzj/go-lua/lib/os/api"
	"github.com/akzj/go-lua/lib/os/internal"
)

// Lib is the OS library instance.
// Use this with vm.OpenLibs().
var Lib osapi.OSLib = internal.NewOSLib()
