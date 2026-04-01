// Package internal implements the string table.
// This package MUST NOT be imported by external modules.
//
// Initialization order:
// 1. mem/internal.init() sets mem/api.DefaultAllocator
// 2. string/internal.init() sets string/api.DefaultStringTable
package internal

import (
	// Import mem to trigger mem/internal.init() which sets DefaultAllocator.
	// This ensures the allocator is ready before any string table uses it.
	_ "github.com/akzj/go-lua/mem"
	"github.com/akzj/go-lua/string/api"
)

func init() {
	// Initialize the default string table for the api package.
	api.DefaultStringTable = NewStringTable()
}
