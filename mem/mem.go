// Package mem provides the Lua VM memory allocator.
//
// This is the root package that imports internal to initialize the
// default allocator before any user code runs.
package mem

import (
	// Import internal to trigger init() which sets api.DefaultAllocator.
	_ "github.com/akzj/go-lua/mem/internal"
)

// Ensure internal package is imported by mem root package.
var _ = struct{}{}
