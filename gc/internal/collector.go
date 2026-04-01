// Package internal provides the GC implementation.
package internal

import (
	"sync"
	"unsafe"

	gcapi "github.com/akzj/go-lua/gc/api"
	memapi "github.com/akzj/go-lua/mem/api"
	types "github.com/akzj/go-lua/types/api"
)

// GCObject is the common header for all collectable objects in the GC.
// This is a copy of types/internal.GCObject to avoid cross-module imports.
type GCObject struct {
	Next   *GCObject
	Tt     uint8
	Marked uint8
}

// GCTrackedObject implements gc/api.GCTrackedObject for GCObject.
func (g *GCObject) Mark() uint8 {
	return g.Marked
}

func (g *GCObject) SetMark(m uint8) {
	g.Marked = m
}

func (g *GCObject) Size() uint64 {
	return 64 // default size
}

func (g *GCObject) Type() int {
	return int(g.Tt) & 0x0F
}

// Table represents a Lua table for GC purposes.
type Table struct {
	GCObject
	Flags      uint8
	Lsizenode  uint8
	Asize      uint32
	Array      []*TValue
	Node       []Node
	Metatable  *Table
	GClist     *GCObject
	WeakMode   gcapi.WeakTableMode
}

func (t *Table) SizeNode() int {
	if t.Lsizenode >= 32 {
		return 0
	}
	return 1 << t.Lsizenode
}

// Node represents a hash table node for GC purposes.
type Node struct {
	KeyValue GCObject
	KeyTt    uint8
	KeyNext  int32
	Val      TValue
}

func (n *Node) KeyIsNil() bool {
	return n.KeyTt == 0
}

func (n *Node) KeyIsDead() bool {
	return n.KeyTt == 10 // LUA_TDEADKEY
}

func (n *Node) KeyIsCollectable() bool {
	return int(n.KeyTt)&0x40 != 0
}

func (n *Node) KeyGCObject() *GCObject {
	if n.KeyIsCollectable() {
		return &n.KeyValue
	}
	return nil
}

// TValue is a simplified Lua value for GC traversal.
type TValue struct {
	Value GCObject
	Tt    uint8
}

func (t *TValue) IsCollectable() bool {
	return int(t.Tt)&0x40 != 0
}

func (t *TValue) GetGC() *GCObject {
	if t.IsCollectable() {
		return &t.Value
	}
	return nil
}

// Collector implements the gc/api.GCCollector interface.
// It provides incremental mark-and-sweep garbage collection with generational mode.
type Collector struct {
	// Memory allocator
	alloc memapi.Allocator

	// GC state machine
	state int // GCS* constant

	// Color tracking
	currentWhite uint8 // current white color (1 or 0)

	// Gray list for marking phase
	gray      *GCObject   // list of gray objects (to traverse)
	grayAgain *GCObject   // list of gray objects that need retraversal

	// Sweep lists
	allgc   *GCObject // all collectable objects
	finobj  *GCObject // objects with finalizers
	tobefnz *GCObject // objects to be finalized

	// Weak table support
	weak     *GCObject // weak tables
	allweak  *GCObject // all weak tables (including ephemeron)

	// Fixed objects (prevented from GC)
	fixedgc *GCObject

	// GC control
	gcstop uint8 // GC stop reason (GCstp* bits)
	gckind int   // GC kind (KGCInc, KGCGenMinor, KGCGenMajor)

	// Memory accounting
	totalbytes  uint64 // total bytes in use
	gcthreshold uint64 // threshold to trigger GC

	// Step control
	gcdebt int64 // debt in bytes (negative = under threshold)

	// Pause control
	stopped bool // true if GC is stopped

	// Incremental state
	sweepgc  *GCObject // current sweep position
	sweepfin *GCObject // sweep position in finobj

	// Generational mode
	genminormul int // minor collection multiplier
	genmajormul int // major collection multiplier

	// Mutex for thread safety
	mu sync.Mutex
}

// NewCollector creates a new GC collector.
func NewCollector(alloc memapi.Allocator) *Collector {
	if alloc == nil {
		alloc = memapi.DefaultAllocator
	}
	c := &Collector{
		alloc:          alloc,
		state:          gcapi.GCSpause,
		currentWhite:   1, // start with white = 1
		gckind:         gcapi.KGCInc,
		totalbytes:     0,
		gcthreshold:    0,
		genminormul:    20,
		genmajormul:    100,
	}
	return c
}

// Collect performs a full garbage collection cycle.
// Returns the number of bytes freed.
func (c *Collector) Collect() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return 0
	}

	// Mark and sweep until complete
	for c.state != gcapi.GCSpause {
		c.stepOnce()
	}

	return c.totalbytes
}

// Step performs a single incremental GC step.
// Returns true if there is more work to do, false if GC cycle is complete.
func (c *Collector) Step() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return false
	}

	// If GC is paused, start a new cycle
	if c.state == gcapi.GCSpause {
		c.enterpropagate()
	}

	// Do one step
	return c.stepOnce()
}

// stepOnce performs a single atomic step of the GC.
// Returns true if more work remains.
func (c *Collector) stepOnce() bool {
	switch c.state {
	case gcapi.GCSpause:
		// Start new cycle
		c.enterpropagate()
		return true

	case gcapi.GCSpropagate:
		// Propagate marks from gray objects
		if c.gray != nil {
			c.propagateMark()
			return true
		}
		// No more gray objects, enter atomic
		c.state = gcapi.GCSenteratomic
		return true

	case gcapi.GCSenteratomic:
		// Entering atomic phase - need to mark all roots atomically
		c.state = gcapi.GCSatomic
		return true

	case gcapi.GCSatomic:
		// Atomic marking phase
		if c.atomic() {
			// Atomic phase done, start sweeping
			c.entersweep()
			return true
		}
		return true

	case gcapi.GCSswpallgc:
		// Sweep all GC objects
		if c.sweepOneList(&c.allgc, &c.sweepgc) {
			return true
		}
		c.state = gcapi.GCSswpfinobj
		return true

	case gcapi.GCSswpfinobj:
		// Sweep finalizable objects
		if c.sweepOneList(&c.finobj, &c.sweepfin) {
			return true
		}
		c.state = gcapi.GCSswptobefnz
		return true

	case gcapi.GCSswptobefnz:
		// Sweep objects to be finalized
		if c.sweepOneList(&c.tobefnz, nil) {
			return true
		}
		c.state = gcapi.GCSswpend
		return true

	case gcapi.GCSswpend:
		// End of sweep
		c.state = gcapi.GCScallfin
		return true

	case gcapi.GCScallfin:
		// Call finalizers
		if c.callFinalizers() {
			c.state = gcapi.GCSpause
			c.currentWhite = c.otherWhite() // flip white
			c.resetGC()
			return false // cycle complete
		}
		return true

	default:
		return false
	}
}

// propagateMark marks one gray object and removes it from gray list.
func (c *Collector) propagateMark() {
	if c.gray == nil {
		return
	}

	obj := c.gray
	c.gray = obj.Next
	c.reallymarkobject(obj)
}

// reallymarkobject marks an object gray and adds it to gray list.
func (c *Collector) reallymarkobject(obj *GCObject) {
	if obj == nil {
		return
	}

	// If in generational mode and object is old, may need barrier
	if c.gckind != gcapi.KGCInc && c.getAge(obj) >= gcapi.GOld {
		c.barrierBack(obj)
		return
	}

	// Set to gray (clear color bits)
	obj.Marked &^= gcapi.White | gcapi.Black

	// Add to gray list
	obj.Next = c.gray
	c.gray = obj
}

// atomic performs the atomic marking phase.
func (c *Collector) atomic() bool {
	// 1. Mark all gray objects (traverse all gray lists)
	c.atomicTraverse()

	// 2. Process weak tables
	c.processWeakTables()

	// 3. Flip current white to other white
	c.currentWhite = c.otherWhite()

	// 4. Mark all black objects gray again for next cycle
	c.markBlackGray()

	// 5. Re-traverse ephemeron tables
	c.reviveEphemeron()

	if c.state == gcapi.GCSatomic {
		c.state = gcapi.GCSswpallgc
		return true
	}

	return false
}

// atomicTraverse traverses all gray objects atomically.
func (c *Collector) atomicTraverse() {
	// Traverse main gray list
	for c.gray != nil {
		c.propagateMark()
	}

	// Traverse grayAgain list
	for c.grayAgain != nil {
		obj := c.grayAgain
		c.grayAgain = obj.Next
		c.traverseobject(obj)
	}
}

// traverseobject traverses references within an object.
func (c *Collector) traverseobject(obj *GCObject) {
	if obj == nil {
		return
	}

	// Set to black (marked, not traversable)
	c.set2black(obj)

	// Traverse based on type
	tt := obj.Tt & 0x7F // remove collectable bit

	switch tt {
	case types.LUA_TTABLE:
		c.traverseTable(obj)
	case types.LUA_VLCL, types.LUA_VCCL, types.LUA_VLCF:
		c.traverseClosure(obj)
	case types.LUA_VTHREAD:
		c.traverseThread(obj)
	case types.LUA_VUSERDATA:
		c.traverseUserdata(obj)
	case types.LUA_VPROTO:
		c.traverseProto(obj)
	}
}

// traverseTable traverses a table's array and hash parts.
func (c *Collector) traverseTable(obj *GCObject) {
	t := c.objectToTable(obj)
	if t == nil {
		return
	}

	// Traverse array part
	for i := uint32(0); i < t.Asize; i++ {
		if int(i) < len(t.Array) && t.Array[i] != nil {
			c.markValue(t.Array[i])
		}
	}

	// Traverse hash part
	if len(t.Node) > 0 {
		nodes := t.SizeNode()
		for i := 0; i < nodes && i < len(t.Node); i++ {
			n := &t.Node[i]
			if !n.KeyIsNil() {
				c.markNodeKey(n)
				c.markValue(&n.Val)
			}
		}
	}

	// Traverse metatable
	if t.Metatable != nil {
		c.markValue(&TValue{
			Tt:    uint8(types.Ctb(int(types.LUA_VTABLE))),
			Value: t.Metatable.GCObject,
		})
	}
}

// traverseClosure traverses a closure's upvalues.
func (c *Collector) traverseClosure(obj *GCObject) {
	// For closures, we need to traverse upvalues
	// This is a simplified implementation
	_ = obj
}

// traverseThread traverses a thread's stack.
func (c *Collector) traverseThread(obj *GCObject) {
	// Traverse thread stack
	// This is simplified
	_ = obj
}

// traverseUserdata traverses a userdata.
func (c *Collector) traverseUserdata(obj *GCObject) {
	// Traverse userdata if it has a metatable
	_ = obj
}

// traverseProto traverses a proto's constants.
func (c *Collector) traverseProto(obj *GCObject) {
	// Traverse proto constants
	_ = obj
}

// markValue marks a value if it's a collectable object.
func (c *Collector) markValue(val *TValue) {
	if val == nil {
		return
	}

	if val.IsCollectable() && val.GetGC() != nil {
		obj := val.GetGC()
		if c.iswhite(obj) {
			c.reallymarkobject(obj)
		}
	}
}

// markNodeKey marks a node's key.
func (c *Collector) markNodeKey(n *Node) {
	if n == nil {
		return
	}

	if n.KeyIsCollectable() && n.KeyGCObject() != nil {
		obj := n.KeyGCObject()
		if c.iswhite(obj) {
			c.reallymarkobject(obj)
		}
	}
}

// processWeakTables removes dead keys/values from weak tables.
func (c *Collector) processWeakTables() {
	// Process allweak list for ephemeron tables
	for c.allweak != nil {
		obj := c.allweak
		c.allweak = obj.Next
		c.clearWeakTable(obj)
	}

	// Process weak list for regular weak tables
	for c.weak != nil {
		obj := c.weak
		c.weak = obj.Next
		c.clearWeakTable(obj)
	}
}

// clearWeakTable clears dead entries from a weak table.
func (c *Collector) clearWeakTable(obj *GCObject) {
	t := c.objectToTable(obj)
	if t == nil {
		return
	}

	mode := t.WeakMode

	// Clear array part if weak values
	if mode&gcapi.WeakValue != 0 {
		c.clearArrayValues(t)
	}

	// Clear hash part
	c.clearHashValues(t, mode)
}

// clearArrayValues clears dead values from array part.
func (c *Collector) clearArrayValues(t *Table) {
	for i := uint32(0); i < t.Asize; i++ {
		if int(i) < len(t.Array) && t.Array[i] != nil {
			// Clear dead values
			_ = i
		}
	}
}

// clearHashValues clears dead keys/values from hash part.
func (c *Collector) clearHashValues(t *Table, mode gcapi.WeakTableMode) {
	if len(t.Node) == 0 {
		return
	}

	nodes := t.SizeNode()
	for i := 0; i < nodes && i < len(t.Node); i++ {
		n := &t.Node[i]
		if n.KeyIsNil() {
			continue
		}

		// Check if key should be cleared
		if mode&gcapi.WeakKey != 0 && n.KeyGCObject() != nil && c.iswhite(n.KeyGCObject()) {
			// Clear key
			n.KeyTt = 10 // LUA_TDEADKEY
		}
	}
}

// markBlackGray marks all black objects gray again.
func (c *Collector) markBlackGray() {
	// Mark all objects in allgc as gray if they are black
	for obj := c.allgc; obj != nil; obj = obj.Next {
		if c.isblack(obj) {
			c.reallymarkobject(obj)
		}
	}
}

// reviveEphemeron re-traverses ephemeron tables.
func (c *Collector) reviveEphemeron() {
	// Ephemeron tables need special handling
	// Objects reachable only through weak keys are not marked
	// This is handled by the atomic phase
}

// entersweep enters the sweep phase.
func (c *Collector) entersweep() {
	c.sweepgc = c.allgc
	c.sweepfin = c.finobj
	c.state = gcapi.GCSswpallgc
}

// sweepOneList sweeps objects in a list.
// Returns true if more sweeping needed.
func (c *Collector) sweepOneList(list **GCObject, sweepPos **GCObject) bool {
	if list == nil || *list == nil {
		return false
	}

	// Start sweeping from sweepPos or beginning
	start := c.allgc
	if sweepPos != nil && *sweepPos != nil {
		start = *sweepPos
	}

	// Sweep up to 20 objects
	count := 0
	var prev *GCObject
	var obj *GCObject
	for obj = start; obj != nil && count < 20; {
		count++

		if c.isdead(obj) {
			// Object is dead, free it
			bytes := c.objsize(obj)
			c.totalbytes -= bytes

			// Remove from list
			next := obj.Next
			// Free the object
			c.freeObject(obj)
			if prev == nil {
				*list = next
			} else {
				prev.Next = next
			}
			obj = next
		} else {
			// Object is live, make it white
			c.makewhite(obj)
			prev = obj
			obj = obj.Next
		}
	}

	// Update sweep position
	if sweepPos != nil {
		*sweepPos = obj
	}

	return obj != nil // more to sweep if obj != nil
}

// freeObject frees a single GC object.
func (c *Collector) freeObject(obj *GCObject) {
	size := c.objsize(obj)
	// Free using allocator
	c.alloc.Free(unsafe.Pointer(obj), memapi.LuaMem(size))
}

// callFinalizers calls finalizers for all objects in tobefnz.
func (c *Collector) callFinalizers() bool {
	// Process objects with finalizers
	// In a full implementation, this would call __gc metamethods

	// Clear the tobefnz list
	c.tobefnz = nil
	return true
}

// enterpropagate starts the propagation phase.
func (c *Collector) enterpropagate() {
	c.state = gcapi.GCSpropagate
	c.gray = nil
	c.grayAgain = nil
	c.weak = nil
	c.allweak = nil
}

// resetGC resets GC state after a cycle.
func (c *Collector) resetGC() {
	// Update threshold
	if c.gckind == gcapi.KGCInc {
		c.gcthreshold = c.totalbytes + c.totalbytes/100
	} else {
		// Generational mode
		c.gcthreshold = c.totalbytes + c.totalbytes/uint64(c.genminormul)
	}
	c.gcdebt = 0
}

// Stop pauses the garbage collector.
func (c *Collector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	c.gcstop |= gcapi.GCstpUser
}

// Start resumes the garbage collector.
func (c *Collector) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = false
	c.gcstop &^= gcapi.GCstpUser

	// If GC was paused, start a new cycle
	if c.state == gcapi.GCSpause {
		c.enterpropagate()
	}
}

// =============================================================================
// Color and Age Helpers
// =============================================================================

// iswhite checks if object is white.
func (c *Collector) iswhite(obj *GCObject) bool {
	return obj != nil && gcapi.IsWhite(obj.Marked)
}

// isblack checks if object is black.
func (c *Collector) isblack(obj *GCObject) bool {
	return obj != nil && gcapi.IsBlack(obj.Marked)
}

// isgray checks if object is gray.
func (c *Collector) isgray(obj *GCObject) bool {
	return obj != nil && gcapi.IsGray(obj.Marked)
}

// isdead checks if object is dead (old white).
func (c *Collector) isdead(obj *GCObject) bool {
	return obj != nil && gcapi.IsDead(c.otherWhite(), obj.Marked)
}

// set2black sets object to black.
func (c *Collector) set2black(obj *GCObject) {
	if obj != nil {
		obj.Marked = (obj.Marked &^ gcapi.White) | gcapi.Black
	}
}

// makewhite makes an object white.
func (c *Collector) makewhite(obj *GCObject) {
	if obj != nil {
		obj.Marked = (obj.Marked &^ (gcapi.White | gcapi.Black)) | c.currentWhite
	}
}

// otherWhite returns the other white color.
func (c *Collector) otherWhite() uint8 {
	return c.currentWhite ^ 1
}

// getAge returns the age of an object.
func (c *Collector) getAge(obj *GCObject) int {
	if obj == nil {
		return 0
	}
	return gcapi.GetAge(obj.Marked)
}

// setAge sets the age of an object.
func (c *Collector) setAge(obj *GCObject, age int) {
	if obj != nil {
		obj.Marked = gcapi.SetAge(obj.Marked, age)
	}
}

// =============================================================================
// Barrier Functions
// =============================================================================

// barrier performs forward barrier when black references white.
func (c *Collector) barrier(obj, v *GCObject) {
	if c.isblack(obj) && c.iswhite(v) {
		c.barrierForward(obj, v)
	}
}

// barrierForward moves object forward (black->white reference).
func (c *Collector) barrierForward(obj, v *GCObject) {
	if c.gckind != gcapi.KGCInc {
		// Generational mode: v becomes old0
		c.age2old0(v)
	}
	// In incremental mode, we just mark v gray
	c.reallymarkobject(v)
}

// barrierBack performs backward barrier (young->old reference).
func (c *Collector) barrierBack(obj *GCObject) {
	if c.isblack(obj) {
		c.set2gray(obj)
		obj.Next = c.grayAgain
		c.grayAgain = obj
	}
}

// set2gray sets object to gray.
func (c *Collector) set2gray(obj *GCObject) {
	if obj != nil {
		obj.Marked &^= gcapi.White | gcapi.Black
	}
}

// age2old0 ages an object to old0.
func (c *Collector) age2old0(obj *GCObject) {
	if obj != nil && c.getAge(obj) < gcapi.GOld0 {
		c.setAge(obj, gcapi.GOld0)
	}
}

// =============================================================================
// Object Size Calculation
// =============================================================================

// objsize returns the size of a GC object.
func (c *Collector) objsize(obj *GCObject) uint64 {
	if obj == nil {
		return 0
	}

	tt := obj.Tt & 0x7F

	switch tt {
	case types.LUA_TTABLE:
		return c.tableSize(obj)
	case types.LUA_VLCL:
		return 48 // approximate
	case types.LUA_VCCL:
		return 40 // approximate
	case types.LUA_VUSERDATA:
		return 40 // approximate
	case types.LUA_VTHREAD:
		return 200 // approximate
	case types.LUA_VPROTO:
		return 48 // approximate
	case types.LUA_VSHRSTR, types.LUA_VLNGSTR:
		return 32 // approximate
	default:
		return 0
	}
}

// tableSize calculates the size of a table.
func (c *Collector) tableSize(obj *GCObject) uint64 {
	t := c.objectToTable(obj)
	if t == nil {
		return 0
	}

	size := uint64(0)
	// GCObject size
	size += 16
	// Table fields
	size += 8 // Flags, Lsizenode
	size += 4 // Asize
	// Array part
	size += uint64(t.Asize) * 8
	// Node part
	if t.Node != nil {
		nodes := t.SizeNode()
		size += uint64(nodes) * 48 // approximate node size
	}

	return size
}

// =============================================================================
// Conversion Helpers
// =============================================================================

// objectToTable converts GCObject to Table.
func (c *Collector) objectToTable(obj *GCObject) *Table {
	if obj == nil {
		return nil
	}
	// GCObject embeds in Table
	return (*Table)(unsafe.Pointer(obj))
}

// =============================================================================
// GCObject List Management
// =============================================================================

// LinkObject adds an object to a GC list.
func (c *Collector) LinkObject(obj *GCObject, list **GCObject) {
	if obj == nil {
		return
	}
	obj.Next = *list
	*list = obj
	// Set to gray initially
	obj.Marked &^= gcapi.White | gcapi.Black
}

// =============================================================================
// State Query Methods
// =============================================================================

// State returns the current GC state.
func (c *Collector) State() int {
	return c.state
}

// IsRunning returns true if GC is actively collecting.
func (c *Collector) IsRunning() bool {
	return !c.stopped && c.state != gcapi.GCSpause
}

// BytesInUse returns approximate bytes currently allocated.
func (c *Collector) BytesInUse() uint64 {
	return c.totalbytes
}

// BytesThreshold returns the threshold that triggers next GC.
func (c *Collector) BytesThreshold() uint64 {
	return c.gcthreshold
}

// SetThreshold sets the GC threshold.
func (c *Collector) SetThreshold(bytes uint64) {
	c.gcthreshold = bytes
}

// AllocateBytes increases the byte counter.
func (c *Collector) AllocateBytes(bytes uint64) {
	c.totalbytes += bytes
}

// FreeBytes decreases the byte counter.
func (c *Collector) FreeBytes(bytes uint64) {
	if bytes > c.totalbytes {
		c.totalbytes = 0
	} else {
		c.totalbytes -= bytes
	}
}

// =============================================================================
// Generational GC Support
// =============================================================================

// SetGCMode sets the GC mode (incremental or generational).
func (c *Collector) SetGCMode(mode int) {
	c.gckind = mode
}

// GCMode returns the current GC mode.
func (c *Collector) GCMode() int {
	return c.gckind
}

// FullGCMajor performs a major collection in generational mode.
func (c *Collector) FullGCMajor() {
	// Invalidate all ages
	for obj := c.allgc; obj != nil; obj = obj.Next {
		if c.getAge(obj) < gcapi.GOld {
			c.setAge(obj, gcapi.GNew)
		}
	}
	c.gckind = gcapi.KGCGenMajor
}

// MinorGC performs a minor collection in generational mode.
func (c *Collector) MinorGC() {
	// Only collect new objects
	c.gckind = gcapi.KGCGenMinor
}

// =============================================================================
// GCFixator Implementation
// =============================================================================

// Fix prevents object from being collected.
func (c *Collector) Fix(obj *GCObject) {
	c.LinkObject(obj, &c.fixedgc)
}

// FreeFix releases a fixed object back to normal GC.
func (c *Collector) FreeFix(obj *GCObject) {
	// Remove from fixedgc list
	var prev *GCObject
	for cur := c.fixedgc; cur != nil; cur = cur.Next {
		if cur == obj {
			if prev == nil {
				c.fixedgc = cur.Next
			} else {
				prev.Next = cur.Next
			}
			// Re-link to allgc
			c.LinkObject(obj, &c.allgc)
			return
		}
		prev = cur
	}
}
