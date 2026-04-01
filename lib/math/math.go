// Package math provides the Lua math library.
//
// This package exports the math library for use by the VM's OpenLibs() function.
//
// Usage:
//
//	import "github.com/akzj/go-lua/lib/math"
//	// In VM initialization:
//	vm.OpenLibs(math.Lib)
package math

import (
	mathapi "github.com/akzj/go-lua/lib/math/api"
	"github.com/akzj/go-lua/lib/math/internal"
)

// Lib is the math library instance.
// Use this with vm.OpenLibs().
var Lib mathapi.MathLib = internal.NewMathLib()