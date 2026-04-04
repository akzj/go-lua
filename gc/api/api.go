// Package api defines the garbage collector interfaces for Lua VM.
// NO dependencies on internal/ packages - pure interface definitions.
//
// Reference: lua-master/lgc.c, lua-master/lgc.h
//
// Design constraints:
// - GCCollector is the only public interface
// - All implementation details live in gc/internal/
// - GC relies on types (TValue, GCObject), mem (Allocator), vm (VMExecutor)
package api

import (
	memapi "github.com/akzj/go-lua/mem/api"
)

// =============================================================================
// GC State Constants
// =============================================================================

// GC state machine states (from lgc.h GCS* constants)
const (
	GCSpropagate   = 0 // propagation phase
	GCSenteratomic = 1 // entering atomic phase
	GCSatomic     = 2 // atomic phase
	GCSswpallgc   = 3 // sweep all GC objects
	GCSswpfinobj  = 4 // sweep finalizable objects
	GCSswptobefnz = 5 // sweep to-be-finalized objects
	GCSswpend     = 6 // end of sweep
	GCScallfin    = 7 // call finalizers
	GCSpause      = 8 // pause/waiting
)

// Object color constants (for marking)
// In Lua: White = bit 0, Black = bit 1 (see lgc.h WHITEBITS, BLACKBIT)
const (
	White = 1 << iota // 1: current white (bit 0)
	Black             // 2: black (bit 1)
)

// Age constants for generational GC (from lgc.h AGEBITS)
const (
	GNew = iota // 0: new object
	GSurvival   // 1: survived one cycle
	GOld0       // 2: old, but just promoted
	GOld1       // 3: old, survived more than one cycle
	GOld        // 4: regular old object
	GTouched1   // 5: touched in current cycle
	GTouched2   // 6: touched in previous cycle
)

// GC kind modes
const (
	KGCInvalid = iota // 0: invalid
	KGCInc           // 1: incremental mode
	KGCGenMinor     // 2: generational minor collection
	KGCGenMajor     // 3: generational major collection
)

// GC stop reasons (gcstp bits from lgc.h)
const (
	GCstpUser = 1 << iota // stopped by user (lua_gc stop)
	GCstpGC               // stopped by GC itself
	GCstpCLS              // stopped by state closing
)

// =============================================================================
// Color/Marking Helpers
// =============================================================================

// IsWhite checks if object is white (collectable).
// Pre: marked must be a marked byte with color bits
func IsWhite(marked uint8) bool {
	return marked&White != 0
}

// IsBlack checks if object is black.
// Pre: marked must be a marked byte with color bits
func IsBlack(marked uint8) bool {
	return marked&Black != 0
}

// IsGray checks if object is gray (neither white nor black).
// Pre: marked must be a marked byte with color bits
func IsGray(marked uint8) bool {
	return marked&(White|Black) == 0
}

// IsDead checks if object is dead (old white).
// Pre: ow is otherwhite, marked is the object's marked byte
func IsDead(ow, marked uint8) bool {
	return marked&3 == ow
}

// GetAge extracts the age bits from marked byte.
func GetAge(marked uint8) int {
	return int(marked >> 3)
}

// SetAge sets the age bits in marked byte.
func SetAge(marked uint8, age int) uint8 {
	return (marked &^ 0x38) | uint8(age<<3)
}

// =============================================================================
// Core Interfaces
// =============================================================================

// GCCollector manages Lua garbage collection.
//
// Invariants:
// - After Stop(), Collect() and Step() must be no-ops until Start()
// - Step() returns true when more GC work remains
// - Collect() performs a complete GC cycle, returns bytes freed
//
// Why these methods?
// - Collect() for explicit full GC (lua_gc(L, GCcollect))
// - Step() for incremental GC (lua_gc(L, GCstep))
// - Stop/Start for pause control (lua_gc(L, GCstop/GCstart))
type GCCollector interface {
	// Collect performs a full garbage collection cycle.
	// Returns the number of bytes freed.
	// Post: returns >= 0
	Collect() uint64

	// Step performs a single incremental GC step.
	// Returns true if there is more work to do, false if GC cycle is complete.
	// Post: returns false only when gcstp != 0 or cycle is done
	Step() bool

	// Stop pauses the garbage collector.
	// Post: subsequent Step() calls return immediately
	Stop()

	// Start resumes the garbage collector.
	// Post: GC resumes from where it was stopped
	Start()

	// AllocateBytes increases the byte counter.
	// Called by allocator on each allocation.
	AllocateBytes(bytes uint64)

	// BytesInUse returns approximate bytes currently allocated.
	BytesInUse() uint64

	// BytesThreshold returns the threshold that triggers next GC.
	BytesThreshold() uint64
}

// GCState provides read access to GC state machine.
// Used by other modules to query GC status.
type GCState interface {
	// State returns the current GC state (GCS* constant).
	State() int

	// IsRunning returns true if GC is actively collecting.
	IsRunning() bool

	// BytesInUse returns approximate bytes currently allocated.
	BytesInUse() uint64

	// BytesThreshold returns the threshold that triggers next GC.
	BytesThreshold() uint64
}

// GCBarrier is called when a black object references a white object.
// This is the forward barrier (luaC_barrier_).
//
// Invariant: caller must hold the lock or be in safe state
type GCBarrier interface {
	// Barrier marks v as referenced by black object o.
	// If in incremental mode, o may turn white to avoid further barriers.
	// Pre: o is black, v is white
	Barrier(o, v interface{})
}

// GCFixator prevents object from being collected.
// Called when creating lua_CFunction upvalues that must survive GC.
type GCFixator interface {
	// Fix prevents object from being collected.
	// The object must be the most recently created (first in allgc).
	// Post: object is removed from allgc and linked to fixedgc
	Fix(object interface{})

	// FreeFix releases a fixed object back to normal GC.
	// Post: object is removed from fixedgc
	FreeFix(object interface{})
}

// =============================================================================
// GC Object Tracking Interfaces
// =============================================================================

// GCTrackedObject is an object that can be tracked by the collector.
type GCTrackedObject interface {
	// Mark returns the current mark byte.
	Mark() uint8

	// SetMark sets the mark byte.
	SetMark(m uint8)

	// Size returns the approximate size in bytes for GC accounting.
	Size() uint64

	// Type returns the Lua type (LUA_T*).
	Type() int
}

// GCObjectList is a doubly-linked list of GC objects.
type GCObjectList interface {
	// Head returns the first object in the list, or nil.
	Head() GCTrackedObject

	// Next returns the next object after obj, or nil.
	Next(obj GCTrackedObject) GCTrackedObject
}

// =============================================================================
// Finalization Interfaces
// =============================================================================

// FinalizableObject is an object with a finalizer (__gc metamethod).
type FinalizableObject interface {
	GCTrackedObject

	// HasFinalizer returns true if object has __gc metamethod.
	HasFinalizer() bool

	// SetFinalizer marks object as having a finalizer.
	SetFinalizer(hasFinalizer bool)

	// MarkFinalized returns true if finalizer has already run.
	MarkFinalized() bool

	// SetMarkFinalized marks the object as finalized.
	SetMarkFinalized()
}

// =============================================================================
// Weak Table Interfaces
// =============================================================================

// WeakTableMode represents weak table configuration.
type WeakTableMode int

const (
	WeakKey   WeakTableMode = 1 << iota // weak keys
	WeakValue                          // weak values
)

// GCWeakTable is a table with weak references.
type GCWeakTable interface {
	// Mode returns the weak table mode.
	Mode() WeakTableMode

	// SetMode sets the weak table mode.
	SetMode(mode WeakTableMode)
}

// =============================================================================
// Barrier Helpers (pure, stateless)
// =============================================================================

// IsCollectable checks if a value is a collectable GC object.
func IsCollectable(tag int) bool {
	return tag&0x40 != 0 // BIT_ISCOLLECTABLE
}

// MakeCollectable sets the collectable bit in a tag.
func MakeCollectable(tag int) int {
	return tag | 0x40
}

// Novariant strips variant bits, returning base type.
func Novariant(tag int) int {
	return tag & 0x0F
}

// Ctb sets the collectable bit (equivalent to C macro).
func Ctb(t int) int {
	return t | 0x40
}

// CurrentWhite returns the current white color for the GC state.
func CurrentWhite(currentWhite uint8) uint8 {
	return currentWhite
}

// OtherWhite returns the other white color (dead white).
func OtherWhite(currentWhite uint8) uint8 {
	return currentWhite ^ 1
}

// =============================================================================
// Factory
// =============================================================================

// DefaultGCCollector is the default GC collector instance.
// Initialized by gc/init.go.
var DefaultGCCollector GCCollector

// NewCollector creates a new garbage collector with the given allocator.
func NewCollector(alloc memapi.Allocator) GCCollector {
	if DefaultGCCollector != nil {
		return DefaultGCCollector
	}
	return nil
}
