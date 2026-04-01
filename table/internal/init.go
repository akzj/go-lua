// Package internal implements the Lua table.
// This package MUST NOT be imported by external modules.
//
// Initialization order:
// 1. mem/internal.init() sets mem/api.DefaultAllocator
// 2. string/internal.init() sets string/api.DefaultStringTable
// 3. table/internal.init() sets table/api.DefaultTable
package internal

import (
	// Import mem to trigger mem/internal.init() which sets DefaultAllocator.
	// This ensures the allocator is ready before any table uses it.
	_ "github.com/akzj/go-lua/mem"
	tableapi "github.com/akzj/go-lua/table/api"
)

func init() {
	// Initialize the default table for the api package.
	// NewTable creates a TableImpl.
	tableapi.DefaultTable = NewTable()
}
