// Package state implements the global state
package state

import (
	"github.com/akzj/go-lua/pkg/object"
)

// GlobalState holds shared state across all VMs
type GlobalState struct {
	// Memory management
	TotalBytes  int64
	GCState     GCState
	GCThreshold int64

	// String interning
	StringTable map[string]*GCString

	// Registry
	Registry *object.Table

	// Version
	Version string
}

// GCState represents garbage collector state
type GCState int

const (
	GCSpause GCState = iota
	GCPropagate
	GCAtomic
	GCEnteratomic
	GCsweepstring
	GCsweep
	GCFinish
)

// GCString is an interned string
type GCString struct {
	Value string
	Hash  uint32
}

// NewGlobalState creates a new global state
func NewGlobalState() *GlobalState {
	return &GlobalState{
		StringTable: make(map[string]*GCString),
		Registry:    object.NewTableWithSize(0, 32),
		Version:     "Go-Lua 0.1.0",
	}
}

// InternString interns a string (returns existing or creates new)
func (gs *GlobalState) InternString(s string) *GCString {
	if gc, ok := gs.StringTable[s]; ok {
		return gc
	}
	gc := &GCString{
		Value: s,
		Hash:  hashString(s),
	}
	gs.StringTable[s] = gc
	return gc
}

// hashString computes a hash for a string
func hashString(s string) uint32 {
	var h uint32
	for i := 0; i < len(s); i++ {
		h = 31*h + uint32(s[i])
	}
	return h
}