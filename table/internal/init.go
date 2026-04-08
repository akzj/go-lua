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
	typesapi "github.com/akzj/go-lua/types/api"
)

func init() {
	// Initialize the default table for the api package.
	// NewTable creates a TableImpl.
	tableapi.DefaultTable = NewTable()
	// Register factory so api.NewTable() creates fresh instances
	tableapi.NewTableFactory = func() tableapi.TableInterface {
		return NewTable()
	}
	// Register WrapRawTable factory so vm/internal can wrap metatables
	tableapi.WrapRawTableFactory = func(t typesapi.Table) tableapi.TableInterface {
		result := WrapRawTable(t)
		if result == nil {
			return nil  // Return untyped nil to avoid non-nil interface wrapping nil pointer
		}
		return result
	}
}
