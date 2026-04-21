package lua

import (
	"github.com/akzj/go-lua/internal/api"
)

// ---------------------------------------------------------------------------
// Garbage collection operations
// ---------------------------------------------------------------------------

// GC performs a garbage collection operation specified by what.
func (L *State) GC(what GCWhat, args ...int) int {
	return L.s.GC(api.GCWhat(what), args...)
}

// GCCollect runs a full garbage collection cycle.
func (L *State) GCCollect() {
	L.s.GCCollect()
}

// GCStepAPI runs a bounded incremental GC step.
// Returns true if a full GC cycle completed during this step.
func (L *State) GCStepAPI() bool {
	return L.s.GCStepAPI()
}


// GCTotalBytes returns the total number of bytes tracked by the Lua GC.
func (L *State) GCTotalBytes() int64 {
	return L.s.GCTotalBytes()
}

// GetGCMode returns the current GC mode ("incremental" or "generational").
func (L *State) GetGCMode() string {
	return L.s.GetGCMode()
}

// SetGCMode sets the GC mode and returns the previous mode.
func (L *State) SetGCMode(mode string) string {
	return L.s.SetGCMode(mode)
}

// SetGCStopped sets or clears the GC stopped flag.
func (L *State) SetGCStopped(stopped bool) {
	L.s.SetGCStopped(stopped)
}

// IsGCRunning returns true if the GC is not stopped.
func (L *State) IsGCRunning() bool {
	return L.s.IsGCRunning()
}

// GetGCParam returns the current value of a GC parameter by name.
// Known parameters: "pause", "stepmul", "stepsize", "minormul", "majorminor", "minormajor".
func (L *State) GetGCParam(name string) int64 {
	return L.s.GetGCParam(name)
}

// SetGCParam sets a GC parameter and returns the previous value.
func (L *State) SetGCParam(name string, value int64) int64 {
	return L.s.SetGCParam(name, value)
}
