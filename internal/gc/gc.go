// Package gc implements Lua's incremental mark-and-sweep garbage collector.
//
// The GC runs as a state machine with the following phases:
//   GCSpause → GCSpropagate → GCSenteratomic → GCSatomic →
//   GCSswpallgc → GCSswpfinobj → GCSswptobefnz → GCSswpend →
//   GCScallfin → GCSpause
//
// Each call to SingleStep does bounded work and advances one step.
// FullGC runs the state machine to completion for stop-the-world collection.
//
// Key difference from C Lua: sweep only unlinks objects from the allgc
// chain. Go's tracing GC handles actual memory deallocation once objects
// are no longer referenced from Go roots.
//
// Reference: lua-master/lgc.c
package gc

import (
	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// Constants for work accounting
// ---------------------------------------------------------------------------

const (
	// SWEEPMAX is the maximum number of objects to sweep per incremental step.
	SWEEPMAX = 20

	// GCFINALIZECOST is the work units charged for calling one finalizer.
	GCFINALIZECOST int64 = 50
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
	g.Gray = append(g.Gray, obj)
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

// genlink handles post-traversal age management for generational GC.
// In gen mode, TOUCHED1 objects go back on grayagain (re-traverse next cycle).
// TOUCHED2 objects advance to OLD.
// In incremental mode, this is a no-op.
// Mirrors C Lua's genlink (lgc.c:470).
func genlink(g *state.GlobalState, obj object.GCObject) {
	if g.GCKind == object.KGC_INC {
		return // no-op in incremental mode
	}
	h := obj.GC()
	if h.Age == object.G_TOUCHED1 {
		// Touched in this cycle — link back to grayagain for next cycle
		g.GrayAgain = append(g.GrayAgain, obj)
	} else if h.Age == object.G_TOUCHED2 {
		// Advance age
		h.Age = object.G_OLD
	}
}

// ---------------------------------------------------------------------------
// Write barriers — maintain the tri-color invariant during mutator execution
// ---------------------------------------------------------------------------

// Barrier is the forward write barrier.
// Called when a black parent object gets a reference to a potentially white child.
// During propagate phase: marks the child (pushes it forward to gray/black).
// This is a no-op if parent is not black or child is not white.
//
// Mirrors C Lua's luaC_barrier_ (lgc.c:246).
func Barrier(g *state.GlobalState, parent, child object.GCObject) {
	if parent == nil || child == nil {
		return
	}
	ph := parent.GC()
	ch := child.GC()
	// Fast path: most objects aren't black during mutator execution
	if ph.Marked&object.BlackBit == 0 || ch.Marked&object.WhiteBits == 0 {
		return
	}
	// Gen mode: if parent is old, promote child to OLD0
	if g.GCKind != object.KGC_INC && ph.IsOld() {
		ch.Age = object.G_OLD0
	}
	// Mark the child (propagate forward)
	markObject(g, child)
}

// BarrierBack is the backward write barrier for container objects (tables).
// Called when a black container is mutated. Sets the container back to gray
// and adds it to the grayagain list for re-traversal in the atomic phase.
// This is a no-op if the parent is not black.
//
// Mirrors C Lua's luaC_barrierback_ (lgc.c:272).
func BarrierBack(g *state.GlobalState, parent object.GCObject) {
	if parent == nil {
		return
	}
	ph := parent.GC()
	// Fast path: not black → nothing to do
	if ph.Marked&object.BlackBit == 0 {
		return
	}
	// Gen mode: if parent is old, set to TOUCHED1
	if g.GCKind != object.KGC_INC && ph.IsOld() {
		ph.Age = object.G_TOUCHED1
	}
	// Set back to gray (clear black bit, keep other bits)
	ph.Marked &^= object.BlackBit
	g.GrayAgain = append(g.GrayAgain, parent)
}

// BarrierValue calls Barrier if val contains a GC-collectable object.
// Convenience wrapper for TValue children.
func BarrierValue(g *state.GlobalState, parent object.GCObject, val object.TValue) {
	if child, ok := val.Obj.(object.GCObject); ok && child != nil {
		Barrier(g, parent, child)
	}
}

// CloseUpvals closes all open upvalues at or above level, applying a forward
// barrier on each closed upvalue. This wraps closure.CloseUpvals logic with
// per-upvalue barriers that closure.go can't call (import cycle: gc→closure).
// Mirrors C Lua's luaF_closeupval + luaC_barrier per upvalue.
func CloseUpvals(g *state.GlobalState, L *state.LuaState, level int) {
	for {
		if L.OpenUpval == nil {
			break
		}
		uv, ok := L.OpenUpval.(*closure.UpVal)
		if !ok || uv == nil || uv.StackIdx < level {
			break
		}

		// Remove from open list
		if uv.Next == nil {
			L.OpenUpval = nil
		} else {
			L.OpenUpval = uv.Next
		}

		// Close: capture the current stack value
		var val object.TValue
		if uv.StackIdx >= 0 && uv.StackIdx < len(L.Stack) {
			val = L.Stack[uv.StackIdx].Val
		} else {
			val = object.Nil
		}
		uv.Close(val)

		// Forward barrier: if upvalue is black and captured value is white
		BarrierValue(g, uv, val)
	}
}


// ---------------------------------------------------------------------------
// Traversal functions — process gray objects, marking their children
// ---------------------------------------------------------------------------

// propagateAll drains the gray list, traversing each gray object.
func propagateAll(g *state.GlobalState) {
	for len(g.Gray) > 0 {
		propagateMark(g)
	}
}

// propagateMark takes one object from the gray list, marks it black,
// and traverses its children. Returns approximate work units (number
// of child slots traversed) for incremental step accounting.
func propagateMark(g *state.GlobalState) int64 {
	// Pop from end of gray slice (stack order)
	n := len(g.Gray)
	obj := g.Gray[n-1]
	g.Gray[n-1] = nil // clear reference to help Go GC
	g.Gray = g.Gray[:n-1]
	h := obj.GC()

	var work int64

	switch v := obj.(type) {
	case *table.Table:
		// Tables may NOT be marked black — weak tables link to special lists.
		// traverseTable handles marking black for strong tables.
		// genlink is called inside traverseTable for strong tables.
		traverseTable(g, v)
		work = int64(len(v.Array) + len(v.Nodes) + 1)
	case *object.Userdata:
		markBlack(h)
		traverseUserdata(g, v)
		genlink(g, obj)
		work = int64(len(v.UserVals) + 1)
	case *closure.LClosure:
		markBlack(h)
		traverseLClosure(g, v)
		work = int64(len(v.UpVals) + 1)
	case *closure.CClosure:
		markBlack(h)
		traverseCClosure(g, v)
		work = int64(len(v.UpVals) + 1)
	case *object.Proto:
		markBlack(h)
		traverseProto(g, v)
		work = int64(len(v.Constants) + len(v.Protos) + len(v.Upvalues) + len(v.LocVars) + 1)
	case *state.LuaState:
		markBlack(h)
		traverseThread(g, v)
		work = int64(len(v.Stack) + 1)
	default:
		markBlack(h)
		work = 1
	}
	return work
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

// linkGCList appends a table to a GC slice (weak, ephemeron, allweak, grayagain).
func linkGCList(t *table.Table, list *[]object.GCObject) {
	*list = append(*list, t)
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
		genlink(g, t) // gen mode: handle TOUCHED1/TOUCHED2 age transitions
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
		genlink(g, t)          // gen mode: handle TOUCHED age transitions
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
		genlink(g, t)          // gen mode: handle TOUCHED age transitions
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
	// In gen mode, old threads must be visited at every cycle because they
	// might point to young objects. In inc mode, threads must be revisited
	// in the atomic phase. Put thread back on grayagain in these cases.
	// Mirrors C Lua's traversethread (lgc.c:697-698).
	h := th.GC()
	if h.IsOld() || g.GCState == object.GCSpropagate {
		g.GrayAgain = append(g.GrayAgain, th)
	}

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
	// Clear gray lists (reuse backing arrays to avoid reallocation)
	g.Gray = g.Gray[:0]
	g.GrayAgain = g.GrayAgain[:0]
	g.Weak = g.Weak[:0]
	g.AllWeak = g.AllWeak[:0]
	g.Ephemeron = g.Ephemeron[:0]

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
				g.GCTotalBytes -= h.ObjSize
			}
			// Remove dead strings from the interning table
			if g.SweepStringFn != nil {
				if _, ok := obj.(*object.LuaString); ok {
					g.SweepStringFn(obj)
				}
			}
			// Return dead objects to pools for reuse
			switch o := obj.(type) {
			case *table.Table:
				table.PutTable(o)
			case *closure.LClosure:
				closure.PutLClosure(o)
			case *closure.UpVal:
				closure.PutUpVal(o)
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

// sweepStep sweeps up to SWEEPMAX objects from the list pointed to by
// g.SweepGC. When the current list is exhausted, advances to nextState.
// Returns approximate work units (count of objects examined).
func sweepStep(g *state.GlobalState, list *object.GCObject, nextState byte) int64 {
	if g.SweepGC == nil {
		// Point SweepGC at the head of this list
		g.SweepGC = list
	}

	otherwhite := otherWhite(g)
	currentWhite := g.CurrentWhite
	count := 0

	for count < SWEEPMAX && *g.SweepGC != nil {
		obj := *g.SweepGC
		h := obj.GC()
		marked := h.Marked

		if isDeadMark(otherwhite, marked) {
			// Dead — unlink from chain
			*g.SweepGC = h.Next
			h.Next = nil
			if h.ObjSize > 0 {
				g.GCTotalBytes -= h.ObjSize
			}
			// Remove dead strings from the interning table
			if g.SweepStringFn != nil {
				if _, ok := obj.(*object.LuaString); ok {
					g.SweepStringFn(obj)
				}
			}
			// Return dead objects to pools for reuse
			switch o := obj.(type) {
			case *table.Table:
				table.PutTable(o)
			case *closure.LClosure:
				closure.PutLClosure(o)
			case *closure.UpVal:
				closure.PutUpVal(o)
			}
		} else {
			// Alive — reset to current white
			h.Marked = (marked &^ (object.WhiteBits | object.BlackBit)) | currentWhite
			g.SweepGC = &h.Next
		}
		count++
	}

	if *g.SweepGC == nil {
		// List exhausted — advance to next state
		g.SweepGC = nil
		g.GCState = nextState
	}

	return int64(count)
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
// Objects are appended to the END of tobefnz to preserve finobj order
// (most recently created first = LIFO finalization order).
// Mirrors C Lua's separatetobefnz which uses findlast + append.
func separateTobeFnz(g *state.GlobalState) {
	otherwhite := otherWhite(g)
	// Find the tail of tobefnz list (to append, not prepend)
	lastnext := &g.TobeFnz
	for *lastnext != nil {
		lastnext = &(*lastnext).GC().Next
	}
	p := &g.FinObj
	for *p != nil {
		obj := *p
		h := obj.GC()
		if isDeadMark(otherwhite, h.Marked) {
			// Dead — move to end of tobefnz
			*p = h.Next
			h.Next = nil
			*lastnext = obj
			lastnext = &h.Next
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
// Weak table clearing functions
// ---------------------------------------------------------------------------

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

func clearByKeys(g *state.GlobalState, list []object.GCObject) {
	for _, obj := range list {
		t := obj.(*table.Table)
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
// Processes list[startIdx:] — entries from startIdx to end of slice.
// Mirrors C Lua's clearbyvalues.
func clearByValues(g *state.GlobalState, list []object.GCObject, startIdx int) {
	for _, obj := range list[startIdx:] {
		t := obj.(*table.Table)

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
	// Scratch buffer reused across iterations to avoid allocation.
	// We swap g.Ephemeron into scratch, then let traverseEphemeron
	// append new entries into g.Ephemeron (which reuses scratch's old capacity).
	var scratch []object.GCObject
	for {
		// Swap: take current list into scratch, give scratch's (empty) backing to g.Ephemeron.
		scratch, g.Ephemeron = g.Ephemeron, scratch[:0]
		if len(scratch) == 0 {
			break
		}
		changed := false
		for _, obj := range scratch {
			t := obj.(*table.Table)
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
// Atomic phase
// ---------------------------------------------------------------------------

// atomic performs the non-interruptible atomic phase of the GC.
// This includes: re-marking the running thread and registry, draining
// grayagain, converging ephemerons, clearing weak tables, separating
// finalizable objects, and flipping the white bit.
// Returns approximate work units.
// Mirrors C Lua's atomic() in lgc.c.
func atomicPhase(g *state.GlobalState, L *state.LuaState) int64 {
	var work int64

	g.GCState = object.GCSatomic

	// Re-mark running thread (may differ from main thread)
	if L != nil {
		markObject(g, L)
	}
	// Re-mark registry
	markValue(g, g.Registry)
	propagateAll(g)
	work += int64(len(g.Gray))

	// Process grayagain list (weak tables deferred from propagate phase)
	work += int64(len(g.GrayAgain))
	g.Gray = append(g.Gray, g.GrayAgain...)
	g.GrayAgain = g.GrayAgain[:0]
	propagateAll(g)

	// Converge ephemerons (weak-key tables with white-key→white-value entries)
	convergeEphemerons(g)

	// All strongly accessible objects are now marked.
	// Clear values from weak-value tables BEFORE checking finalizers.
	clearByValues(g, g.Weak, 0)
	clearByValues(g, g.AllWeak, 0)
	origWeakLen := len(g.Weak)
	origAllLen := len(g.AllWeak)

	// Separate dead finalizable objects
	separateTobeFnz(g)
	// Mark objects being finalized (resurrection)
	markBeingFnz(g)
	propagateAll(g)
	// Second ephemeron convergence (for resurrected objects)
	convergeEphemerons(g)

	// Clear dead keys from ephemeron and allweak tables
	clearDeadKeysAllEphemerons(g)
	clearByKeys(g, g.AllWeak)
	// Clear values from resurrected weak tables (only new entries since pre-resurrection)
	clearByValues(g, g.Weak, origWeakLen)
	clearByValues(g, g.AllWeak, origAllLen)

	// Update GCEstimate with current live bytes
	g.GCEstimate = g.GCTotalBytes

	return work
}

// entersweep sets up the sweep phase by pointing SweepGC at the allgc list.
func entersweep(g *state.GlobalState) {
	g.GCState = object.GCSswpallgc
	g.SweepGC = &g.Allgc
}

// ---------------------------------------------------------------------------
// State machine — SingleStep
// ---------------------------------------------------------------------------

// SingleStep performs one incremental step of the GC state machine.
// Returns approximate work units done in this step.
// Mirrors C Lua's singlestep() in lgc.c.
func SingleStep(g *state.GlobalState, L *state.LuaState) int64 {
	switch g.GCState {
	case object.GCSpause:
		// Reset ephemeron counter for this cycle
		g.EphemeronCount = 0
		// Flip white at cycle start (restartcollection).
		// Objects created before this point have the OLD white → candidates.
		// Objects created during mark get the NEW white → survive this cycle.
		// A second flip happens at the end of atomic() to prepare for sweep.
		g.CurrentWhite ^= object.WhiteBits
		// Mark roots, enter propagate
		markRoots(g)
		g.GCState = object.GCSpropagate
		return 1

	case object.GCSpropagate:
		if len(g.Gray) > 0 {
			// Propagate one gray object
			return propagateMark(g)
		}
		// Gray list empty → enter atomic
		g.GCState = object.GCSenteratomic
		return 0

	case object.GCSenteratomic:
		// Do the entire atomic phase (non-interruptible)
		work := atomicPhase(g, L)
		// Set up sweep
		entersweep(g)
		return work

	case object.GCSswpallgc:
		// Sweep some objects from allgc
		return sweepStep(g, &g.Allgc, object.GCSswpfinobj)

	case object.GCSswpfinobj:
		// Sweep some objects from finobj
		return sweepStep(g, &g.FinObj, object.GCSswptobefnz)

	case object.GCSswptobefnz:
		// Sweep some objects from tobefnz
		return sweepStep(g, &g.TobeFnz, object.GCSswpend)

	case object.GCSswpend:
		// Sweep finished — clear gray lists, enter callfin
		g.Gray = g.Gray[:0]
		g.GrayAgain = g.GrayAgain[:0]
		g.Weak = g.Weak[:0]
		g.AllWeak = g.AllWeak[:0]
		g.Ephemeron = g.Ephemeron[:0]
		g.GCState = object.GCScallfin
		return 0

	case object.GCScallfin:
		// Finalizer calling is handled externally by the API layer
		// (callAllPendingFinalizers). In the state machine, we just
		// transition to pause. The API layer checks TobeFnz separately.
		g.GCState = object.GCSpause
		return 0

	default:
		// Should never happen — treat as pause
		g.GCState = object.GCSpause
		return 0
	}
}

// ---------------------------------------------------------------------------
// Full GC cycle
// ---------------------------------------------------------------------------

// FullGC performs a complete garbage collection cycle by running the
// state machine to completion. This is the Go equivalent of C Lua's
// fullinc (luaC_fullgc).
//
// If a cycle is already in progress (state > GCSpause and <= GCSpropagate),
// it completes the current cycle first, then runs a fresh one.
func FullGC(g *state.GlobalState, L *state.LuaState) {
	// If we're in the middle of a mark phase, we need to finish the
	// current cycle first before starting a new one.
	if g.GCState <= object.GCSpropagate {
		// Already in mark phase or pause — finish current cycle
		// by running through all states until pause
		for g.GCState != object.GCSpause {
			SingleStep(g, L)
		}
	}

	// Run one complete cycle: from pause through all states back to pause
	for {
		SingleStep(g, L)
		if g.GCState == object.GCSpause {
			break
		}
	}

	// Clear SweepGC for safety (cycle is complete)
	g.SweepGC = nil
}

// ---------------------------------------------------------------------------
// GC Step + Debt-Based Pacing
// ---------------------------------------------------------------------------

// GCStep performs bounded incremental GC work triggered by the debt-based
// pacer. Matches C Lua's incstep() (lgc.c:1710-1728):
//
//	stepsize = applygcparam(STEPSIZE, 100)
//	work2do  = applygcparam(STEPMUL, stepsize / sizeof(void*))
//	fast     = (work2do == 0)   // stepsize=0 → full collection per step
//	loop SingleStep until work >= work2do or cycle completes (or fast)
//	if completed: SetPause resets debt based on live data
//	if partial:   debt = stepsize (allocation credit until next step)
func GCStep(g *state.GlobalState, L *state.LuaState) {
	if g.GCStopped {
		return
	}

	// GCStepSize is log2 of step size in bytes (default 13 → 8KB).
	// Special case: stepsize=0 means "stop-the-world" — run a full cycle
	// per step (matches C Lua's fast flag in incstep).
	if g.GCStepSize == 0 {
		FullGC(g, L)
		SetPause(g)
		return
	}

	// Calculate work budget.
	// stepsize = 1 << GCStepSize (bytes → work units, divided by pointer size)
	// work2do  = stepmul * (stepsize / ptrsize) / 100
	// C Lua divides by sizeof(void*) to convert bytes to "work units".
	stepsize := int64(1) << g.GCStepSize // e.g. 1<<13 = 8192
	stepmul := int64(g.GCStepMul)
	if stepmul == 0 {
		stepmul = 200 // safety default
	}
	work2do := stepmul * (stepsize / 8) / 100 // 8 = sizeof(void*) on 64-bit

	// Do bounded incremental work via SingleStep
	var work int64
	for {
		work += SingleStep(g, L)
		if g.GCState == object.GCSpause || work >= work2do {
			break
		}
	}

	if g.GCState == object.GCSpause {
		// Completed a full cycle — recalculate debt based on live data
		SetPause(g)
	} else {
		// Partial step — set debt = stepsize bytes of allocation credit
		// before the next GC step triggers.
		g.GCDebt = stepsize
	}
}

// SetPause calculates the GC debt (allocation credit) after a collection.
// The debt determines how many bytes can be allocated before the next
// GC cycle triggers. Based on C Lua's setpause():
//   threshold = estimate * pause / 100
//   debt = threshold - current_total_bytes
// With default pause=200, GC triggers when memory reaches 2x live data.
func SetPause(g *state.GlobalState) {
	estimate := g.GCTotalBytes
	if estimate < 1 {
		estimate = 1
	}
	g.GCEstimate = estimate

	pause := g.GCPause
	if pause < 2 {
		pause = 2 // minimum pause to avoid constant collection
	}

	// threshold = estimate * pause / 100
	// debt = threshold - estimate (right after GC, total ≈ estimate)
	threshold := estimate * int64(pause) / 100
	debt := threshold - estimate
	// Minimum debt to avoid thrashing on small heaps
	const minDebt int64 = 64 * 1024
	if debt < minDebt {
		debt = minDebt
	}
	g.GCDebt = debt
}
