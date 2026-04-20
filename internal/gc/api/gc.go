// Package api implements Lua's mark-and-sweep garbage collector.
//
// This is a stop-the-world, non-incremental GC. It implements the core
// mark/sweep/finalize cycle from C Lua's lgc.c, adapted for Go.
//
// Key difference from C Lua: sweep only unlinks objects from the allgc
// chain. Go's tracing GC handles actual memory deallocation once objects
// are no longer referenced from Go roots.
//
// Reference: lua-master/lgc.c
package api

import (
	closureapi "github.com/akzj/go-lua/internal/closure/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
	tableapi "github.com/akzj/go-lua/internal/table/api"
)

// ---------------------------------------------------------------------------
// Mark helpers
// ---------------------------------------------------------------------------

// markObject marks an object as gray (to be traversed) if it is currently white.
// Strings and closed upvalues are marked black immediately (no children to traverse).
func markObject(g *stateapi.GlobalState, obj objectapi.GCObject) {
	if obj == nil {
		return
	}
	h := obj.GC()
	if !h.IsWhite() {
		return // already marked
	}

	switch v := obj.(type) {
	case *objectapi.LuaString:
		// Strings have no children — mark black immediately
		h.Marked = (h.Marked &^ objectapi.WhiteBits) | objectapi.BlackBit
		_ = v
	case *closureapi.UpVal:
		if v.IsOpen() {
			// Open upvalues: the stack value is marked via traverseThread.
			// Keep gray — value may change before atomic phase.
			h.Marked &^= (objectapi.WhiteBits | objectapi.BlackBit) // gray
		} else {
			// Closed upvalues: mark the owned value, then go black.
			markValue(g, v.Own)
			h.Marked = (h.Marked &^ objectapi.WhiteBits) | objectapi.BlackBit
		}
	case *objectapi.Userdata:
		if len(v.UserVals) == 0 {
			// No user values — mark metatable and go black
			if mt, ok := v.MetaTable.(*tableapi.Table); ok && mt != nil {
				markObject(g, mt)
			}
			h.Marked = (h.Marked &^ objectapi.WhiteBits) | objectapi.BlackBit
		} else {
			// Has user values — add to gray list for traversal
			linkGray(g, obj)
		}
	default:
		// Table, LClosure, CClosure, LuaState, Proto — add to gray list
		linkGray(g, obj)
	}
}

// linkGray adds an object to the gray list for later traversal.
func linkGray(g *stateapi.GlobalState, obj objectapi.GCObject) {
	h := obj.GC()
	// Clear white bits (now gray — neither white nor black)
	h.Marked &^= (objectapi.WhiteBits | objectapi.BlackBit)
	h.GCList = g.Gray
	g.Gray = obj
}

// markValue marks the value inside a TValue if it's a collectable object.
func markValue(g *stateapi.GlobalState, tv objectapi.TValue) {
	if obj, ok := tv.Val.(objectapi.GCObject); ok {
		markObject(g, obj)
	}
}

// markBlack marks an object as black (fully traversed).
func markBlack(h *objectapi.GCHeader) {
	h.Marked = (h.Marked &^ objectapi.WhiteBits) | objectapi.BlackBit
}

// ---------------------------------------------------------------------------
// Traversal functions — process gray objects, marking their children
// ---------------------------------------------------------------------------

// propagateAll drains the gray list, traversing each gray object.
func propagateAll(g *stateapi.GlobalState) {
	for g.Gray != nil {
		propagateMark(g)
	}
}

// propagateMark takes one object from the gray list, marks it black,
// and traverses its children.
func propagateMark(g *stateapi.GlobalState) {
	obj := g.Gray
	h := obj.GC()
	// Remove from gray list
	g.Gray = h.GCList
	h.GCList = nil
	// Mark black
	markBlack(h)

	switch v := obj.(type) {
	case *tableapi.Table:
		traverseTable(g, v)
	case *objectapi.Userdata:
		traverseUserdata(g, v)
	case *closureapi.LClosure:
		traverseLClosure(g, v)
	case *closureapi.CClosure:
		traverseCClosure(g, v)
	case *objectapi.Proto:
		traverseProto(g, v)
	case *stateapi.LuaState:
		traverseThread(g, v)
	}
}

// traverseTable marks all references in a table.
// For now, treats all tables as strong (no weak table handling).
func traverseTable(g *stateapi.GlobalState, t *tableapi.Table) {
	// Mark metatable
	if t.Metatable != nil {
		markObject(g, t.Metatable)
	}

	// Mark array part
	for i := range t.Array {
		markValue(g, t.Array[i])
	}

	// Mark hash part
	nodes := t.Nodes
	for i := range nodes {
		n := &nodes[i]
		// Mark value if slot is occupied (not empty/nil)
		if n.Val.Tt != objectapi.TagEmpty && n.Val.Tt != objectapi.TagNil {
			markValue(g, n.Val)
			// Mark key
			if obj, ok := n.KeyVal.(objectapi.GCObject); ok {
				markObject(g, obj)
			}
		}
	}
}

// traverseUserdata marks metatable and user values.
func traverseUserdata(g *stateapi.GlobalState, u *objectapi.Userdata) {
	if mt, ok := u.MetaTable.(*tableapi.Table); ok && mt != nil {
		markObject(g, mt)
	}
	for i := range u.UserVals {
		markValue(g, u.UserVals[i])
	}
}

// traverseLClosure marks the proto and all upvalues.
func traverseLClosure(g *stateapi.GlobalState, cl *closureapi.LClosure) {
	if cl.Proto != nil {
		markObject(g, cl.Proto)
	}
	for _, uv := range cl.UpVals {
		if uv != nil {
			markObject(g, uv)
		}
	}
}

// traverseCClosure marks all upvalues (stored as TValues).
func traverseCClosure(g *stateapi.GlobalState, cl *closureapi.CClosure) {
	for i := range cl.UpVals {
		markValue(g, cl.UpVals[i])
	}
}

// traverseProto marks source string, constants, nested protos,
// upvalue names, and local variable names.
func traverseProto(g *stateapi.GlobalState, p *objectapi.Proto) {
	if p.Source != nil {
		markObject(g, p.Source)
	}
	for i := range p.Constants {
		markValue(g, p.Constants[i])
	}
	for _, child := range p.Protos {
		if child != nil {
			markObject(g, child)
		}
	}
	for i := range p.Upvalues {
		if p.Upvalues[i].Name != nil {
			markObject(g, p.Upvalues[i].Name)
		}
	}
	for i := range p.LocVars {
		if p.LocVars[i].Name != nil {
			markObject(g, p.LocVars[i].Name)
		}
	}
}

// traverseThread marks all live stack values and open upvalues.
func traverseThread(g *stateapi.GlobalState, th *stateapi.LuaState) {
	// Mark live stack slots (up to Top)
	for i := 0; i < th.Top && i < len(th.Stack); i++ {
		markValue(g, th.Stack[i].Val)
	}

	// Also mark stack slots used by active call frames above Top
	// (call frames may reference slots between their Base and Top)
	for ci := th.CI; ci != nil; ci = ci.Prev {
		if ci.Top > th.Top {
			for i := th.Top; i < ci.Top && i < len(th.Stack); i++ {
				markValue(g, th.Stack[i].Val)
			}
		}
	}

	// Mark open upvalues
	if th.OpenUpval != nil {
		if uv, ok := th.OpenUpval.(*closureapi.UpVal); ok {
			for uv != nil {
				markObject(g, uv)
				uv = uv.Next
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Mark roots
// ---------------------------------------------------------------------------

// markRoots marks all GC root objects.
func markRoots(g *stateapi.GlobalState) {
	// Clear gray lists
	g.Gray = nil
	g.GrayAgain = nil
	g.Weak = nil
	g.AllWeak = nil
	g.Ephemeron = nil

	// Mark main thread
	if g.MainThread != nil {
		markObject(g, g.MainThread)
	}

	// Mark registry
	markValue(g, g.Registry)

	// Mark global metatables
	for i := 0; i < len(g.MT); i++ {
		if t, ok := g.MT[i].(*tableapi.Table); ok && t != nil {
			markObject(g, t)
		}
	}

	// Mark TM name strings
	for _, s := range g.TMNames {
		if s != nil {
			markObject(g, s)
		}
	}

	// Mark MemErrMsg
	if g.MemErrMsg != nil {
		markObject(g, g.MemErrMsg)
	}

	// Mark objects being finalized (tobefnz list)
	for obj := g.TobeFnz; obj != nil; obj = obj.GC().Next {
		markObject(g, obj)
	}
}

// ---------------------------------------------------------------------------
// Sweep
// ---------------------------------------------------------------------------

// sweepList walks a GC chain starting at *p, removing dead objects
// (those with the "other" white color) and flipping live objects to
// the current white. Returns the count of freed objects.
//
// In Go, "freeing" means unlinking from the chain. Go's tracing GC
// will collect the memory once no more Go references exist.
func sweepList(g *stateapi.GlobalState, p *objectapi.GCObject) int {
	otherwhite := otherWhite(g)
	currentWhite := g.CurrentWhite
	freed := 0

	for *p != nil {
		obj := *p
		h := obj.GC()
		marked := h.Marked

		if isDeadMark(otherwhite, marked) {
			// Dead — unlink from chain
			*p = h.Next
			h.Next = nil // help Go GC
			freed++
		} else {
			// Alive — reset to current white for next cycle
			h.Marked = (marked &^ (objectapi.WhiteBits | objectapi.BlackBit)) | currentWhite
			p = &h.Next
		}
	}
	return freed
}

// otherWhite returns the "other" white bit (the one NOT current).
func otherWhite(g *stateapi.GlobalState) byte {
	return g.CurrentWhite ^ objectapi.WhiteBits
}

// isDeadMark checks if a mark byte indicates a dead object.
func isDeadMark(otherwhite byte, marked byte) bool {
	// An object is dead if it has the "other" white color
	// (i.e., it was white at the start of the cycle and never marked)
	return marked&objectapi.WhiteBits == otherwhite && marked&objectapi.BlackBit == 0
}

// ---------------------------------------------------------------------------
// Finalization support
// ---------------------------------------------------------------------------

// separateTobeFnz moves dead objects from finobj to tobefnz.
// These objects have __gc metamethods and need finalization.
func separateTobeFnz(g *stateapi.GlobalState) {
	otherwhite := otherWhite(g)
	p := &g.FinObj
	for *p != nil {
		obj := *p
		h := obj.GC()
		if isDeadMark(otherwhite, h.Marked) {
			// Dead — move to tobefnz
			*p = h.Next
			h.Next = g.TobeFnz
			g.TobeFnz = obj
		} else {
			p = &h.Next
		}
	}
}

// ---------------------------------------------------------------------------
// Full GC cycle
// ---------------------------------------------------------------------------

// FullGC performs a complete stop-the-world garbage collection cycle.
// This is the Go equivalent of C Lua's fullinc (luaC_fullgc).
//
// Phases:
//  1. Mark — traverse from roots, color all reachable objects
//  2. Atomic — finalize mark phase, handle weak tables (TODO)
//  3. Sweep — remove dead objects from allgc chain
//  4. Finalize — move dead finobj to tobefnz, call __gc
//  5. Flip — switch current white for next cycle
func FullGC(g *stateapi.GlobalState, L *stateapi.LuaState) {
	// Phase 1: Mark roots
	g.GCState = objectapi.GCSpropagate
	markRoots(g)

	// Phase 2: Propagate — drain gray list
	propagateAll(g)

	// Phase 3: Atomic — re-mark running thread, registry, metatables
	g.GCState = objectapi.GCSatomic
	if L != nil {
		markObject(g, L) // mark running thread (may differ from main)
	}
	markValue(g, g.Registry)
	propagateAll(g) // propagate any new gray objects

	// Flip current white at end of atomic phase (before sweep).
	// Objects that were not marked still have the OLD white.
	// Sweep will identify them as dead (they have "otherwhite").
	// Mirrors C Lua: atomic() flips currentwhite before sweep.
	g.CurrentWhite ^= objectapi.WhiteBits

	// Phase 4: Sweep — dead objects have the old white (now "otherwhite")
	g.GCState = objectapi.GCSswpallgc
	sweepList(g, &g.Allgc)

	// Phase 5: Handle finalizable objects
	g.GCState = objectapi.GCSswpfinobj
	separateTobeFnz(g)
	sweepList(g, &g.FinObj)

	// Phase 6: Reset to pause
	g.GCState = objectapi.GCSpause
}
