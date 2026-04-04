// Package gc provides initialization for the garbage collector.
// The init function creates the default collector instance.
package gc

import (
	gcapi "github.com/akzj/go-lua/gc/api"
	gcinternal "github.com/akzj/go-lua/gc/internal"
	memapi "github.com/akzj/go-lua/mem/api"
)

func init() {
	// Initialize the default GC collector
	gcapi.DefaultGCCollector = gcinternal.NewCollector(memapi.DefaultAllocator)
}
