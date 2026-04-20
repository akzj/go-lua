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

	switch v := obj.(type) {
	case *tableapi.Table:
		// Tables may NOT be marked black — weak tables link to special lists.
		// traverseTable handles marking black for strong tables.
		traverseTable(g, v)
	case *objectapi.Userdata:
		markBlack(h)
		traverseUserdata(g, v)
	case *closureapi.LClosure:
		markBlack(h)
		traverseLClosure(g, v)
	case *closureapi.CClosure:
		markBlack(h)
		traverseCClosure(g, v)
	case *objectapi.Proto:
		markBlack(h)
		traverseProto(g, v)
	case *stateapi.LuaState:
		markBlack(h)
		traverseThread(g, v)
	default:
		markBlack(h)
	}
}

// getWeakMode returns the weak mode of a table (0=strong, 1=weak values,
// 2=weak keys, 3=both). Mirrors C Lua's getmode().
func getWeakMode(t *tableapi.Table) byte {
	return t.WeakMode
}

// isCleared checks if a GCObject value is "cleared" (dead — has the old white).
// Used to determine if weak table entries should be removed.
// Only objects with the "other" white (dead white) are considered cleared.
func isCleared(g *stateapi.GlobalState, obj objectapi.GCObject) bool {
	if obj == nil {
		return false // nil is never "cleared"
	}
	// Strings are never collected by weak table logic (they're values)
	if _, ok := obj.(*objectapi.LuaString); ok {
		return false
	}
	h := obj.GC()
	otherwhite := g.CurrentWhite ^ objectapi.WhiteBits
	return h.Marked&objectapi.WhiteBits == otherwhite && h.Marked&objectapi.BlackBit == 0
}

// linkGCList links a table into a GC list (weak, ephemeron, allweak, grayagain)
// using the GCList field of its GCHeader.
func linkGCList(t *tableapi.Table, list *objectapi.GCObject) {
	t.GCHeader.GCList = *list
	*list = t
}

// traverseTable marks references in a table, handling weak modes.
// Mirrors C Lua's traversetable → traversestrongtable/traverseweakvalue/traverseephemeron.
func traverseTable(g *stateapi.GlobalState, t *tableapi.Table) {
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
	case tableapi.WeakValue: // weak values only
		traverseWeakValue(g, t)
		// Do NOT mark black — stays in weak/grayagain list
	case tableapi.WeakKey: // weak keys only (ephemeron)
		traverseEphemeron(g, t, false)
		// Do NOT mark black — stays in ephemeron/allweak list
	default: // both weak keys and weak values (mode == 3)
		// Don't mark any entries. Link to grayagain (propagate phase)
		// or allweak (atomic phase) for later clearing.
		if g.GCState == objectapi.GCSpropagate {
			linkGCList(t, &g.GrayAgain) // revisit in atomic phase
		} else {
			linkGCList(t, &g.AllWeak) // clear in atomic phase
		}
		// Do NOT mark black
	}
}

// traverseStrongTable marks all keys and values in a strong table.
func traverseStrongTable(g *stateapi.GlobalState, t *tableapi.Table) {
	// Mark array part
	for i := range t.Array {
		markValue(g, t.Array[i])
	}
	// Mark hash part
	nodes := t.Nodes
	for i := range nodes {
		n := &nodes[i]
		if n.Val.Tt != objectapi.TagEmpty && n.Val.Tt != objectapi.TagNil {
			markValue(g, n.Val)
			if obj, ok := n.KeyVal.(objectapi.GCObject); ok {
				markObject(g, obj)
			}
		}
	}
}

// traverseWeakValue marks keys but NOT values of a weak-value table.
// Links the table to g.Weak (atomic phase) or g.GrayAgain (propagate phase).
func traverseWeakValue(g *stateapi.GlobalState, t *tableapi.Table) {
	hasClears := len(t.Array) > 0 // array part may have white values
	// Mark keys in hash part (keys are strong in weak-value tables)
	nodes := t.Nodes
	for i := range nodes {
		n := &nodes[i]
		if n.Val.Tt == objectapi.TagEmpty || n.Val.Tt == objectapi.TagNil {
			continue // empty slot
		}
		// Mark key (strong)
		if obj, ok := n.KeyVal.(objectapi.GCObject); ok {
			markObject(g, obj)
		}
		// Check if value is white (don't mark it — it's weak)
		if !hasClears {
			if vObj, ok := n.Val.Val.(objectapi.GCObject); ok {
				if isCleared(g, vObj) {
					hasClears = true
				}
			}
		}
	}
	if g.GCState == objectapi.GCSpropagate {
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
func traverseEphemeron(g *stateapi.GlobalState, t *tableapi.Table, inv bool) bool {
	hasClears := false // has white keys
	hasWW := false     // has white-key → white-value entries
	marked := false    // marked some new value

	// Traverse array part (always mark — array indices are integers, always alive)
	for i := range t.Array {
		if vObj, ok := t.Array[i].Val.(objectapi.GCObject); ok {
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
		if n.Val.Tt == objectapi.TagEmpty || n.Val.Tt == objectapi.TagNil {
			continue // empty slot
		}
		// Check if key is marked
		keyObj, keyIsGC := n.KeyVal.(objectapi.GCObject)
		keyIsWhite := keyIsGC && isCleared(g, keyObj)
		if keyIsWhite {
			hasClears = true
			// Key is white — check if value is also white
			if vObj, ok := n.Val.Val.(objectapi.GCObject); ok {
				if isCleared(g, vObj) {
					hasWW = true // white→white entry
				}
			}
		} else {
			// Key is marked (or not a GC object) — mark value
			if vObj, ok := n.Val.Val.(objectapi.GCObject); ok {
				if isCleared(g, vObj) {
					markObject(g, vObj)
					marked = true
				}
			}
		}
	}

	// Link table into proper list
	if g.GCState == objectapi.GCSpropagate {
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
	// Mark ALL allocated stack slots. During VM execution, registers
	// above L.Top or CI.Top may hold live values that haven't been
	// cleared yet (e.g., a table just assigned to register ra before
	// periodic GC fires). Marking the full stack is safe because
	// unused slots are nil (markValue skips nil TValues).
	for i := 0; i < len(th.Stack); i++ {
		markValue(g, th.Stack[i].Val)
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

// CheckFinalizer moves an object from the allgc list to the finobj list
// if it has a finalizer (__gc metamethod). Called from SetMetatable when
// __gc is detected. Mirrors C Lua's luaC_checkfinalizer.
//
// The hasFinalizer parameter indicates whether the object has __gc;
// the caller (API layer) is responsible for checking the metatable.
func CheckFinalizer(g *stateapi.GlobalState, obj objectapi.GCObject) {
	h := obj.GC()
	// Already marked as having a finalizer? Skip.
	if h.Marked&objectapi.FinalizedBit != 0 {
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
	h.Marked = g.CurrentWhite | objectapi.FinalizedBit
}

// Udata2Finalize removes the first object from the tobefnz list and
// links it back into allgc. Clears the FinalizedBit so the object is
// "normal" again (can be re-finalized if it gets a new __gc).
// Returns nil if tobefnz is empty.
// Mirrors C Lua's udata2finalize.
func Udata2Finalize(g *stateapi.GlobalState) objectapi.GCObject {
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
	h.Marked &^= objectapi.FinalizedBit
	// Make it white (current white) so it survives this cycle
	h.Marked = (h.Marked &^ (objectapi.WhiteBits | objectapi.BlackBit)) | g.CurrentWhite
	return o
}

// ---------------------------------------------------------------------------
// Full GC cycle
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Weak table clearing functions (Phase B)
// ---------------------------------------------------------------------------

// clearByKeys clears entries with unmarked keys from weak-key tables.
// Walks the given GC list (linked via GCList/gclist field).
// Mirrors C Lua's clearbykeys.
func clearByKeys(g *stateapi.GlobalState, l objectapi.GCObject) {
	for l != nil {
		t := l.(*tableapi.Table)
		l = t.GCHeader.GCList // next in list

		nodes := t.Nodes
		for i := range nodes {
			n := &nodes[i]
			if n.Val.Tt == objectapi.TagEmpty || n.Val.Tt == objectapi.TagNil {
				continue
			}
			// Check if key is a dead GC object
			if keyObj, ok := n.KeyVal.(objectapi.GCObject); ok {
				if isCleared(g, keyObj) {
					// Dead key — clear the entry
					n.Val = objectapi.Nil
				}
			}
			// If entry is now empty, clear the key too
			if n.Val.Tt == objectapi.TagEmpty || n.Val.Tt == objectapi.TagNil {
				n.KeyVal = nil
			}
		}
	}
}

// clearByValues clears entries with unmarked values from weak-value tables.
// Walks the GC list from l up to (but not including) f.
// Mirrors C Lua's clearbyvalues.
func clearByValues(g *stateapi.GlobalState, l objectapi.GCObject, f objectapi.GCObject) {
	for l != nil && l != f {
		t := l.(*tableapi.Table)
		l = t.GCHeader.GCList // next in list

		// Clear array part
		for i := range t.Array {
			if vObj, ok := t.Array[i].Val.(objectapi.GCObject); ok {
				if isCleared(g, vObj) {
					t.Array[i] = objectapi.Nil
				}
			}
		}
		// Clear hash part
		nodes := t.Nodes
		for i := range nodes {
			n := &nodes[i]
			if n.Val.Tt == objectapi.TagEmpty || n.Val.Tt == objectapi.TagNil {
				continue
			}
			if vObj, ok := n.Val.Val.(objectapi.GCObject); ok {
				if isCleared(g, vObj) {
					n.Val = objectapi.Nil
				}
			}
			if n.Val.Tt == objectapi.TagEmpty || n.Val.Tt == objectapi.TagNil {
				n.KeyVal = nil
			}
		}
	}
}

// convergeEphemerons iterates ephemeron tables until no new marks are produced.
// An ephemeron table has weak keys: if a key is marked, its value should be marked.
// This process may need multiple passes because marking a value may mark
// keys in other ephemeron tables.
// Mirrors C Lua's convergeephemerons.
func convergeEphemerons(g *stateapi.GlobalState) {
	dir := false
	for {
		next := g.Ephemeron
		g.Ephemeron = nil
		changed := false
		for next != nil {
			t := next.(*tableapi.Table)
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
func markBeingFnz(g *stateapi.GlobalState) {
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
func FullGC(g *stateapi.GlobalState, L *stateapi.LuaState) {
	// Flip current white BEFORE marking. This is critical for correctness
	// when FullGC runs during VM execution (periodic GC):
	// - Existing objects have the OLD white → must be marked to survive
	// - New objects created during the cycle get the NEW white → survive sweep
	// - Sweep kills objects with OLD white that weren't marked
	g.CurrentWhite ^= objectapi.WhiteBits

	// Phase 1: Mark roots
	g.GCState = objectapi.GCSpropagate
	markRoots(g)

	// Phase 2: Propagate — drain gray list
	propagateAll(g)

	// Phase 3: Atomic — mirrors C Lua's atomic()
	g.GCState = objectapi.GCSatomic
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
	clearByKeys(g, g.Ephemeron)
	clearByKeys(g, g.AllWeak)
	// Clear values from resurrected weak tables (only new entries since origWeak)
	clearByValues(g, g.Weak, origWeak)
	clearByValues(g, g.AllWeak, origAll)

	// Phase 4: Sweep — dead objects have the old white (now "otherwhite")
	g.GCState = objectapi.GCSswpallgc
	sweepList(g, &g.Allgc)

	// Phase 5: Sweep finalizable objects
	g.GCState = objectapi.GCSswpfinobj
	sweepList(g, &g.FinObj)

	// Phase 6: Reset
	g.GCState = objectapi.GCSpause
	// Clear weak table lists for next cycle
	g.Weak = nil
	g.AllWeak = nil
	g.Ephemeron = nil
}
