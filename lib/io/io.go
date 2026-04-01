// Package io provides the Lua I/O library.
//
// This package exports the I/O library for use by the VM's OpenLibs() function.
//
// Usage:
//
//	import "github.com/akzj/go-lua/lib/io"
//	// In VM initialization:
//	vm.OpenLibs(io.Lib)
package io

import (
	ioapi "github.com/akzj/go-lua/lib/io/api"
	"github.com/akzj/go-lua/lib/io/internal"
)

// Lib is the I/O library instance.
// Use this with vm.OpenLibs().
var Lib ioapi.IoLib = internal.NewIoLib()
