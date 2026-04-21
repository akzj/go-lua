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
package gc

import (
	"sync/atomic"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// Mark helpers
// ---------------------------------------------------------------------------

// markObject marks an object as gray (to be traversed) if it is currently white.
// Strings and closed upvalues are marked black immediately (no children to traverse).
func markObject(g *state.GlobalState, obj object.GCObject) {
	if obj == nil {
		return
	}
	h := obj.GC()
	if !h.IsWhite() {
		return // already marked
	}

	switch v := obj.(type) {
	case *object.LuaString:
		// Strings have no children — mark black immediately
		h.Marked = (h.Marked &^ object.WhiteBits) | object.BlackBit
		_ = v
	case *closure.UpVal:
		if v.IsOpen() {
			// Open upvalues: the stack value is marked via traverseThread.
			// Keep gray — value may change before atomic phase.
			h.Marked &^= (object.WhiteBits | object.BlackBit) // gray
		} else {
			// Closed upvalues: mark the owned value, then go black.
			markValue(g, v.Own)
			h.Marked = (h.Marked &^ object.WhiteBits) | object.BlackBit
		}
	case *object.Userdata:
		if len(v.UserVals) == 0 {
			// No user values — mark metatable and go black
			if mt, ok := v.MetaTable.(*table.Table); ok && mt != nil {
				markObject(g, mt)
			}
			h.Marked = (h.Marked &^ object.WhiteBits) | object.BlackBit
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
func linkGray(g *state.GlobalState, obj object.GCObject) {
	h := obj.GC()
	// Clear white bits (now gray — neither white nor black)
	h.Marked &^= (object.WhiteBits | object.BlackBit)
	h.GCList = g.Gray
	g.Gray = obj
}

// markValue marks the value inside a TValue if it's a collectable object.
func markValue(g *state.GlobalState, tv object.TValue) {
	if obj, ok := tv.Obj.(object.GCObject); ok {
		markObject(g, obj)
	}
}

// markBlack marks an object as black (fully traversed).
func markBlack(h *object.GCHeader) {
	h.Marked = (h.Marked &^ object.WhiteBits) | object.BlackBit
}

// ---------------------------------------------------------------------------
// Traversal functions — process gray objects, marking their children
// ---------------------------------------------------------------------------

// propagateAll drains the gray list, traversing each gray object.
func propagateAll(g *state.GlobalState) {
	for g.Gray != nil {
		propagateMark(g)
	}
}

// propagateMark takes one object from the gray list, marks it black,
// and traverses its children.
func propagateMark(g *state.GlobalState) {
	obj := g.Gray
	h := obj.GC()
	// Remove from gray list
	g.Gray = h.GCList
	h.GCList = nil

	switch v := obj.(type) {
	case *table.Table:
		// Tables may NOT be marked black — weak tables link to special lists.
		// traverseTable handles marking black for strong tables.
		traverseTable(g, v)
	case *object.Userdata:
		markBlack(h)
		traverseUserdata(g, v)
	case *closure.LClosure:
		markBlack(h)
		traverseLClosure(g, v)
	case *closure.CClosure:
		markBlack(h)
		traverseCClosure(g, v)
	case *object.Proto:
		markBlack(h)
		traverseProto(g, v)
	case *state.LuaState:
		markBlack(h)
		traverseThread(g, v)
	default:
		markBlack(h)
	}
}

// getWeakMode returns the weak mode of a table (0=strong, 1=weak values,
// 2=weak keys, 3=both). Mirrors C Lua's getmode().
func getWeakMode(t *table.Table) byte {
	return t.WeakMode
}

// isCleared checks if a GCObject value is "cleared" (dead — has the old white).
// Used to determine if weak table entries should be removed.
// Only objects with the "other" white (dead white) are considered cleared.
func isCleared(g *state.GlobalState, obj object.GCObject) bool {
	if obj == nil {
		return false // nil is never "cleared"
	}
	// Strings are never collected by weak table logic (they're values)
	if _, ok := obj.(*object.LuaString); ok {
		return false
	}
	h := obj.GC()
	otherwhite := g.CurrentWhite ^ object.WhiteBits
	return h.Marked&object.WhiteBits == otherwhite && h.Marked&object.BlackBit == 0
}

// linkGCList links a table into a GC list (weak, ephemeron, allweak, grayagain)
// using the GCList field of its GCHeader.
func linkGCList(t *table.Table, list *object.GCObject) {
	t.GCHeader.GCList = *list
	*list = t
}

// traverseTable marks references in a table, handling weak modes.
// Mirrors C Lua's traversetable → traversestrongtable/traverseweakvalue/traverseephemeron.
func traverseTable(g *state.GlobalState, t *table.Table) {
	h := &t.GCHeader
	// Always mark metatable
	if t.Metatable != nil {
		markObject(g, t.Metatable)
	}

	mode := getWeakMode(t)
	switch mode {
	case 0: // not weak — strong table
		traverseStrongTable(g, t)
		markBlack(h)
	case table.WeakValue: // weak values only
		traverseWeakValue(g, t)
		// Do NOT mark black — stays in weak/grayagain list
	case table.WeakKey: // weak keys only (ephemeron)
		traverseEphemeron(g, t, false)
		g.EphemeronCount++ // track for clearDeadKeysAllEphemerons optimization
		// Do NOT mark black — stays in ephemeron/allweak list
	default: // both weak keys and weak values (mode == 3)
		g.EphemeronCount++ // track for clearDeadKeysAllEphemerons optimization
		// Don't mark any entries. Link to grayagain (propagate phase)
		// or allweak (atomic phase) for later clearing.
		if g.GCState == object.GCSpropagate {
			linkGCList(t, &g.GrayAgain) // revisit in atomic phase
		} else {
			linkGCList(t, &g.AllWeak) // clear in atomic phase
		}
		// Do NOT mark black
	}
}

// traverseStrongTable marks all keys and values in a strong table.
func traverseStrongTable(g *state.GlobalState, t *table.Table) {
	// Mark array part
	for i := range t.Array {
		markValue(g, t.Array[i])
	}
	// Mark hash part
	nodes := t.Nodes
	for i := range nodes {
		n := &nodes[i]
		if n.Val.Tt != object.TagEmpty && n.Val.Tt != object.TagNil {
			markValue(g, n.Val)
			if obj, ok := n.KeyVal.(object.GCObject); ok {
				markObject(g, obj)
			}
		}
	}
}

// traverseWeakValue marks keys but NOT values of a weak-value table.
// Links the table to g.Weak (atomic phase) or g.GrayAgain (propagate phase).
func traverseWeakValue(g *state.GlobalState, t *table.Table) {
	hasClears := len(t.Array) > 0 // array part may have white values
	// Mark keys in hash part (keys are strong in weak-value tables)
	nodes := t.Nodes
	for i := range nodes {
		n := &nodes[i]
		if n.Val.Tt == object.TagEmpty || n.Val.Tt == object.TagNil {
			continue // empty slot
		}
		// Mark key (strong)
		if obj, ok := n.KeyVal.(object.GCObject); ok {
			markObject(g, obj)
		}
		// Check if value is white (don't mark it — it's weak)
		if !hasClears {
			if vObj, ok := n.Val.Obj.(object.GCObject); ok {
				if isCleared(g, vObj) {
					hasClears = true
				}
			}
		}
	}
	if g.GCState == object.GCSpropagate {
		linkGCList(t, &g.GrayAgain) // must retraverse in atomic phase
	} else if hasClears {
		linkGCList(t, &g.Weak) // has dead values to clear
	} else {
		markBlack(&t.GCHeader) // no clears needed — done
	}
}

// traverseEphemeron traverses a weak-key (ephemeron) table.
// If a key is marked, its value is marked. If a key is white, the value is NOT marked.
// Returns true if any new value was marked during this traversal.
func traverseEphemeron(g *state.GlobalState, t *table.Table, inv bool) bool {
	hasClears := false // has white keys
	hasWW := false     // has white-key → white-value entries
	marked := false    // marked some new value

	// Traverse array part (always mark — array indices are integers, always alive)
	for i := range t.Array {
		if vObj, ok := t.Array[i].Obj.(object.GCObject); ok {
			if isCleared(g, vObj) {
				// Array values in ephemeron tables: mark them (keys are integers = strong)
				markObject(g, vObj)
				marked = true
			}
		}
	}

	// Traverse hash part
	nodes := t.Nodes
	nsize := len(nodes)
	for idx := 0; idx < nsize; idx++ {
		i := idx
		if inv {
			i = nsize - 1 - idx
		}
		n := &nodes[i]
		if n.Val.Tt == object.TagEmpty || n.Val.Tt == object.TagNil {
			continue // empty slot
		}
		// Check if key is marked
		keyObj, keyIsGC := n.KeyVal.(object.GCObject)
		keyIsWhite := keyIsGC && isCleared(g, keyObj)
		if keyIsWhite {
			hasClears = true
			// Key is white — check if value is also white
			if vObj, ok := n.Val.Obj.(object.GCObject); ok {
				if isCleared(g, vObj) {
					hasWW = true // white→white entry
				}
			}
		} else {
			// Key is marked (or not a GC object) — mark value
			if vObj, ok := n.Val.Obj.(object.GCObject); ok {
				if isCleared(g, vObj) {
					markObject(g, vObj)
					marked = true
				}
			}
		}
	}

	// Link table into proper list
	if g.GCState == object.GCSpropagate {
		linkGCList(t, &g.GrayAgain) // must retraverse in atomic phase
	} else if hasWW {
		linkGCList(t, &g.Ephemeron) // has white→white, need convergence
	} else if hasClears {
		linkGCList(t, &g.AllWeak) // has white keys to clear
	} else {
		markBlack(&t.GCHeader) // fully processed
	}
	return marked
}

// traverseUserdata marks metatable and user values.
func traverseUserdata(g *state.GlobalState, u *object.Userdata) {
	if mt, ok := u.MetaTable.(*table.Table); ok && mt != nil {
		markObject(g, mt)
	}
	for i := range u.UserVals {
		markValue(g, u.UserVals[i])
	}
}

// traverseLClosure marks the proto and all upvalues.
func traverseLClosure(g *state.GlobalState, cl *closure.LClosure) {
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
func traverseCClosure(g *state.GlobalState, cl *closure.CClosure) {
	for i := range cl.UpVals {
		markValue(g, cl.UpVals[i])
	}
}

// traverseProto marks source string, constants, nested protos,
// upvalue names, and local variable names.
func traverseProto(g *state.GlobalState, p *object.Proto) {
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
// Marks up to maxTop (highest of Top and all CI.Top values).
// This is conservative enough for periodic GC (live registers are within
// active frames) while not marking the entire allocated stack (which would
// prevent weak tables from collecting dead locals above all active frames).
func traverseThread(g *state.GlobalState, th *state.LuaState) {
	var top int
	if g.GCExplicit {
		// Explicit GC (collectgarbage()): precise — only up to th.Top.
		// Dead locals above Top in caller frames are NOT marked, allowing
		// weak tables to properly collect them.
		top = th.Top
	} else {
		// Periodic GC (during VM execution): conservative — mark all
		// registers in active frames. The VM may have live values in
		// registers above Top that haven't been pushed yet.
		top = th.Top
		for ci := th.CI; ci != nil; ci = ci.Prev {
			if ci.Top > top {
				top = ci.Top
			}
		}
	}
	if top > len(th.Stack) {
		top = len(th.Stack)
	}
	for i := 0; i < top; i++ {
		markValue(g, th.Stack[i].Val)
	}

	// Mark open upvalues
	if th.OpenUpval != nil {
		if uv, ok := th.OpenUpval.(*closure.UpVal); ok {
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
func markRoots(g *state.GlobalState) {
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
		if t, ok := g.MT[i].(*table.Table); ok && t != nil {
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
func sweepList(g *state.GlobalState, p *object.GCObject) int {
	otherwhite := otherWhite(g)
	currentWhite := g.CurrentWhite
	freed := 0

	for *p != nil {
		obj := *p
		h := obj.GC()
		marked := h.Marked

		if isDeadMark(otherwhite, marked) {
			// Dead — unlink from chain and decrement GCTotalBytes
			*p = h.Next
			h.Next = nil // help Go GC
			// Decrement byte counter using pre-computed size (avoids type assertion)
			if h.ObjSize > 0 {
				atomic.AddInt64(&g.GCTotalBytes, -h.ObjSize)
			}
			// Return dead tables to the pool for reuse
			if t, ok := obj.(*table.Table); ok {
				table.PutTable(t)
			}
			freed++
		} else {
			// Alive — reset to current white for next cycle
			h.Marked = (marked &^ (object.WhiteBits | object.BlackBit)) | currentWhite
			p = &h.Next
		}
	}
	return freed
}

// otherWhite returns the "other" white bit (the one NOT current).
func otherWhite(g *state.GlobalState) byte {
	return g.CurrentWhite ^ object.WhiteBits
}

// isDeadMark checks if a mark byte indicates a dead object.
func isDeadMark(otherwhite byte, marked byte) bool {
	// An object is dead if it has the "other" white color
	// (i.e., it was white at the start of the cycle and never marked)
	return marked&object.WhiteBits == otherwhite && marked&object.BlackBit == 0
}

// ---------------------------------------------------------------------------
// Finalization support
// ---------------------------------------------------------------------------

// separateTobeFnz moves dead objects from finobj to tobefnz.
// These objects have __gc metamethods and need finalization.
func separateTobeFnz(g *state.GlobalState) {
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

// CheckFinalizer moves an object from the allgc list to the finobj list
// if it has a finalizer (__gc metamethod). Called from SetMetatable when
// __gc is detected. Mirrors C Lua's luaC_checkfinalizer.
//
// The hasFinalizer parameter indicates whether the object has __gc;
// the caller (API layer) is responsible for checking the metatable.
func CheckFinalizer(g *state.GlobalState, obj object.GCObject) {
	h := obj.GC()
	// Already marked as having a finalizer? Skip.
	if h.Marked&object.FinalizedBit != 0 {
		return
	}
	// Remove from allgc list
	p := &g.Allgc
	for *p != nil {
		if *p == obj {
			*p = h.Next
			break
		}
		p = &(*p).GC().Next
	}
	// Link into finobj list
	h.Next = g.FinObj
	g.FinObj = obj
	// Set FinalizedBit and ensure the object has the current white color.
	// This is critical: separateTobeFnz identifies dead objects by checking
	// for the "other" white. Without current white, the object can't be
	// detected as dead in the next cycle.
	h.Marked = g.CurrentWhite | object.FinalizedBit
}

// Udata2Finalize removes the first object from the tobefnz list and
// links it back into allgc. Clears the FinalizedBit so the object is
// "normal" again (can be re-finalized if it gets a new __gc).
// Returns nil if tobefnz is empty.
// Mirrors C Lua's udata2finalize.
func Udata2Finalize(g *state.GlobalState) object.GCObject {
	o := g.TobeFnz
	if o == nil {
		return nil
	}
	h := o.GC()
	// Remove from tobefnz
	g.TobeFnz = h.Next
	// Link back into allgc (resurrection)
	h.Next = g.Allgc
	g.Allgc = o
	// Clear finalized bit — object is "normal" again
	h.Marked &^= object.FinalizedBit
	// Make it white (current white) so it survives this cycle
	h.Marked = (h.Marked &^ (object.WhiteBits | object.BlackBit)) | g.CurrentWhite
	return o
}

// ---------------------------------------------------------------------------
// Full GC cycle
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Weak table clearing functions
// ---------------------------------------------------------------------------

// clearByKeys clears entries with unmarked keys from weak-key tables.
// Walks the given GC list (linked via GCList/gclist field).
// Mirrors C Lua's clearbykeys.
// clearDeadKeysAllEphemerons walks allgc and finobj lists to find ALL tables
// with WeakKey mode and clears their dead key entries. This is needed because
// convergeEphemerons empties g.Ephemeron (marks tables black and removes them),
// so clearByKeys(g, g.Ephemeron) would operate on an empty list.
//
// Optimization: if no ephemeron tables were encountered during mark phase
// (EphemeronCount == 0), skip the expensive full-chain walk entirely.
func clearDeadKeysAllEphemerons(g *state.GlobalState) {
	if g.EphemeronCount == 0 {
		return // no weak-key tables this cycle — nothing to clear
	}
	clearDeadKeysInList(g, g.Allgc)
	clearDeadKeysInList(g, g.FinObj)
}

// clearDeadKeysInList walks a GC object list and clears dead keys from any
// table with WeakKey mode set.
func clearDeadKeysInList(g *state.GlobalState, list object.GCObject) {
	for obj := list; obj != nil; obj = obj.GC().Next {
		t, ok := obj.(*table.Table)
		if !ok || t.WeakMode&table.WeakKey == 0 {
			continue
		}
		nodes := t.Nodes
		for i := range nodes {
			n := &nodes[i]
			if n.Val.Tt == object.TagEmpty || n.Val.Tt == object.TagNil {
				continue
			}
			if keyObj, ok := n.KeyVal.(object.GCObject); ok {
				if isCleared(g, keyObj) {
					n.KeyTT = object.TagDeadKey
					n.Val = object.Nil
				}
			}
		}
	}
}

func clearByKeys(g *state.GlobalState, l object.GCObject) {
	for l != nil {
		t := l.(*table.Table)
		l = t.GCHeader.GCList // next in list

		nodes := t.Nodes
		for i := range nodes {
			n := &nodes[i]
			if n.Val.Tt == object.TagEmpty || n.Val.Tt == object.TagNil {
				continue
			}
			// Check if key is a dead GC object
			if keyObj, ok := n.KeyVal.(object.GCObject); ok {
				if isCleared(g, keyObj) {
					// Dead key — mark as dead and clear value
					// Mirrors C Lua's setdeadkey(n)
					n.KeyTT = object.TagDeadKey
					n.Val = object.Nil
				}
			}
		}
	}
}

// clearByValues clears entries with unmarked values from weak-value tables.
// Walks the GC list from l up to (but not including) f.
// Mirrors C Lua's clearbyvalues.
func clearByValues(g *state.GlobalState, l object.GCObject, f object.GCObject) {
	for l != nil && l != f {
		t := l.(*table.Table)
		l = t.GCHeader.GCList // next in list

		// Clear array part
		for i := range t.Array {
			if vObj, ok := t.Array[i].Obj.(object.GCObject); ok {
				if isCleared(g, vObj) {
					t.Array[i] = object.Nil
				}
			}
		}
		// Clear hash part
		nodes := t.Nodes
		for i := range nodes {
			n := &nodes[i]
			if n.Val.Tt == object.TagEmpty || n.Val.Tt == object.TagNil {
				continue
			}
			if vObj, ok := n.Val.Obj.(object.GCObject); ok {
				if isCleared(g, vObj) {
					// Dead value — mark key as dead and clear value.
					// Must NOT nil KeyVal — hash chain probing depends on it.
					n.KeyTT = object.TagDeadKey
					n.Val = object.Nil
				}
			}
		}
	}
}

// convergeEphemerons iterates ephemeron tables until no new marks are produced.
// An ephemeron table has weak keys: if a key is marked, its value should be marked.
// This process may need multiple passes because marking a value may mark
// keys in other ephemeron tables.
// Mirrors C Lua's convergeephemerons.
func convergeEphemerons(g *state.GlobalState) {
	dir := false
	for {
		next := g.Ephemeron
		g.Ephemeron = nil
		changed := false
		for next != nil {
			t := next.(*table.Table)
			next = t.GCHeader.GCList
			// Mark black temporarily (out of list)
			markBlack(&t.GCHeader)
			if traverseEphemeron(g, t, dir) {
				propagateAll(g) // propagate new marks
				changed = true
			}
		}
		dir = !dir // alternate direction
		if !changed {
			break
		}
	}
}

// markBeingFnz marks all objects in the tobefnz list.
// These objects are about to be finalized — they and their references
// must survive this GC cycle (resurrection).
// Mirrors C Lua's markbeingfnz.
func markBeingFnz(g *state.GlobalState) {
	for obj := g.TobeFnz; obj != nil; obj = obj.GC().Next {
		markObject(g, obj)
	}
}

// ---------------------------------------------------------------------------
// Full GC cycle
// ---------------------------------------------------------------------------

// FullGC performs a complete stop-the-world garbage collection cycle.
// This is the Go equivalent of C Lua's fullinc (luaC_fullgc).
//
// Phases:
//  1. Flip white — switch current white so new objects survive
//  2. Mark — traverse from roots, color all reachable objects
//  3. Atomic — process grayagain, ephemerons, weak tables
//  4. Sweep — remove dead objects from allgc chain
//  5. Finalize — move dead finobj to tobefnz
//  6. Reset
func FullGC(g *state.GlobalState, L *state.LuaState) {
	// Flip current white BEFORE marking. This is critical for correctness
	// when FullGC runs during VM execution (periodic GC):
	// - Existing objects have the OLD white → must be marked to survive
	// - New objects created during the cycle get the NEW white → survive sweep
	// - Sweep kills objects with OLD white that weren't marked
	g.CurrentWhite ^= object.WhiteBits

	// Reset ephemeron counter for this cycle (used to skip clearDeadKeysAllEphemerons)
	g.EphemeronCount = 0

	// Phase 1: Mark roots
	g.GCState = object.GCSpropagate
	markRoots(g)

	// Phase 2: Propagate — drain gray list
	propagateAll(g)

	// Phase 3: Atomic — mirrors C Lua's atomic()
	g.GCState = object.GCSatomic
	if L != nil {
		markObject(g, L) // mark running thread (may differ from main)
	}
	markValue(g, g.Registry)
	propagateAll(g) // propagate any new gray objects

	// Process grayagain list (weak tables deferred from propagate phase)
	grayagain := g.GrayAgain
	g.GrayAgain = nil
	g.Gray = grayagain
	propagateAll(g)

	// Converge ephemerons (weak-key tables with white-key→white-value entries)
	convergeEphemerons(g)

	// At this point, all strongly accessible objects are marked.
	// Clear values from weak-value tables BEFORE checking finalizers.
	clearByValues(g, g.Weak, nil)
	clearByValues(g, g.AllWeak, nil)
	origWeak := g.Weak
	origAll := g.AllWeak

	// Separate dead finalizable objects
	separateTobeFnz(g)
	// Mark objects being finalized (resurrection)
	markBeingFnz(g)
	propagateAll(g)
	// Second ephemeron convergence (for resurrected objects)
	convergeEphemerons(g)

	// At this point, all resurrected objects are marked.
	// Clear dead keys from ephemeron and allweak tables
	// NOTE: convergeEphemerons empties g.Ephemeron (marks tables black).
	// Walk allgc+finobj to find ALL ephemeron tables for dead-key clearing.
	clearDeadKeysAllEphemerons(g)
	clearByKeys(g, g.AllWeak)
	// Clear values from resurrected weak tables (only new entries since origWeak)
	clearByValues(g, g.Weak, origWeak)
	clearByValues(g, g.AllWeak, origAll)

	// Phase 4: Sweep — dead objects have the old white (now "otherwhite")
	g.GCState = object.GCSswpallgc
	sweepList(g, &g.Allgc)

	// Phase 5: Sweep finalizable objects
	g.GCState = object.GCSswpfinobj
	sweepList(g, &g.FinObj)

	// Phase 6: Reset
	g.GCState = object.GCSpause
	// Clear weak table lists for next cycle
	g.Weak = nil
	g.AllWeak = nil
	g.Ephemeron = nil
}
