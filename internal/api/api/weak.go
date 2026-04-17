// Weak table support — API layer.
//
// Registers WeakRefMake/WeakRefCheck callbacks for the table package,
// and provides SweepWeakTables for collectgarbage integration.
//
// This package can import all type packages (closure, state, table, object)
// so it handles the concrete weak.Pointer[T] type switches.
package api

import (
	"runtime"
	"weak"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
	tableapi "github.com/akzj/go-lua/internal/table/api"
)

func init() {
	tableapi.WeakRefMake = weakRefMake
	tableapi.WeakRefCheck = weakRefCheck
}

// weakRef holds a weak.Pointer[T] (as any) plus the original tag and value.
// We store the original TValue so we can restore it if the ref is still alive.
type weakRef struct {
	tag objectapi.Tag
	ptr any          // weak.Pointer[T] for some concrete T
	val objectapi.TValue // original value for restoration
}

// weakRefMake creates a weak reference for a pointer-backed TValue.
func weakRefMake(v objectapi.TValue) (any, bool) {
	var ptr any
	switch v.Tt {
	case objectapi.TagTable:
		p := v.Val.(*tableapi.Table)
		ptr = weak.Make(p)
	case objectapi.TagLuaClosure:
		p := v.Val.(*closureapi.LClosure)
		ptr = weak.Make(p)
	case objectapi.TagCClosure:
		p := v.Val.(*closureapi.CClosure)
		ptr = weak.Make(p)
	case objectapi.TagUserdata:
		p := v.Val.(*objectapi.Userdata)
		ptr = weak.Make(p)
	case objectapi.TagThread:
		p := v.Val.(*stateapi.LuaState)
		ptr = weak.Make(p)
	default:
		return nil, false // non-pointer type
	}
	return &weakRef{tag: v.Tt, ptr: ptr, val: v}, true
}

// weakRefCheck checks if a weak reference is still alive.
// Returns the original TValue if alive, or (Nil, false) if collected.
func weakRefCheck(ref any) (objectapi.TValue, bool) {
	wr := ref.(*weakRef)
	switch wr.tag {
	case objectapi.TagTable:
		if wr.ptr.(weak.Pointer[tableapi.Table]).Value() != nil {
			return wr.val, true
		}
	case objectapi.TagLuaClosure:
		if wr.ptr.(weak.Pointer[closureapi.LClosure]).Value() != nil {
			return wr.val, true
		}
	case objectapi.TagCClosure:
		if wr.ptr.(weak.Pointer[closureapi.CClosure]).Value() != nil {
			return wr.val, true
		}
	case objectapi.TagUserdata:
		if wr.ptr.(weak.Pointer[objectapi.Userdata]).Value() != nil {
			return wr.val, true
		}
	case objectapi.TagThread:
		if wr.ptr.(weak.Pointer[stateapi.LuaState]).Value() != nil {
			return wr.val, true
		}
	}
	return objectapi.Nil, false
}

// SweepWeakTables performs the two-phase weak table sweep:
// Phase 1: create weak refs, nil out strong refs
// Phase 2: GC, then check weak refs and restore/clear
func (L *State) SweepWeakTables() {
	gs := L.ls().Global
	if len(gs.WeakTables) == 0 {
		return
	}

	// Phase 1: prepare all weak tables (create weak refs, nil strong refs)
	restoreFns := make([]func(), 0, len(gs.WeakTables))
	for _, wt := range gs.WeakTables {
		t := wt.(*tableapi.Table)
		restore := t.PrepareWeakSweep()
		restoreFns = append(restoreFns, restore)
	}

	// GC to collect unreferenced objects
	runtime.GC()

	// Phase 2: check weak refs and restore/clear
	for _, restore := range restoreFns {
		restore()
	}

	// Prune tables that are no longer weak
	alive := gs.WeakTables[:0]
	for _, wt := range gs.WeakTables {
		t := wt.(*tableapi.Table)
		if t.WeakMode != 0 {
			alive = append(alive, t)
		}
	}
	gs.WeakTables = alive
}
