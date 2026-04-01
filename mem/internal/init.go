// Package internal implements the memory allocator.
// Implementation details hidden from external packages.
package internal

import (
	"github.com/akzj/go-lua/mem/api"
)

func init() {
	// Initialize the default allocator for the api package.
	api.DefaultAllocator = NewAllocator()
}
