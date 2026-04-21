// gc_impl.go — GC integration (gcMarkSweep, GCCollect, clearStaleStack, finalizers).
package api

import (
	"sync/atomic"

	"github.com/akzj/go-lua/internal/gc"
	"github.com/akzj/go-lua/internal/metamethod"
	"github.com/akzj/go-lua/internal/object"

	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// GC
// ---------------------------------------------------------------------------

// GC performs a garbage collection operation.
func (L *State) GC(what GCWhat, args ...int) int {
	// Go's GC handles this. No-op for most operations.
	return 0
}

// GCTotalBytes returns the Lua-level allocation counter (bytes).
// Mirrors C Lua's gettotalbytes(g) for collectgarbage("count").
// Uses atomic load since finalizers may concurrently modify the counter.
func (L *State) GCTotalBytes() int64 {
	return atomic.LoadInt64(&L.ls().Global.GCTotalBytes)
}

// TrackAlloc adds n bytes to the Lua-level allocation counter.
// Uses atomic add since finalizers may concurrently modify the counter.
func (L *State) TrackAlloc(n int64) {
	atomic.AddInt64(&L.ls().Global.GCTotalBytes, n)
}

// GetGCMode returns the current GC mode string ("incremental" or "generational").
// Defaults to "incremental" if not set.
func (L *State) GetGCMode() string {
	mode := L.ls().Global.GCMode
	if mode == "" {
		return "incremental"
	}
	return mode
}

// SetGCMode sets the GC mode and returns the previous mode string.
func (L *State) SetGCMode(mode string) string {
	prev := L.GetGCMode()
	L.ls().Global.GCMode = mode
	return prev
}

// SetGCStopped sets or clears the GCStopped flag (collectgarbage "stop"/"restart").
func (L *State) SetGCStopped(stopped bool) {
	L.ls().Global.GCStopped = stopped
}

// IsGCRunning returns true if GC is not stopped.
func (L *State) IsGCRunning() bool {
	return !L.ls().Global.GCStopped
}

// IsGCInFinalizer returns true if a __gc finalizer is currently executing.
func (L *State) IsGCInFinalizer() bool {
	return L.ls().Global.GCRunningFinalizer
}

// gcMarkSweep runs mark/sweep only — no finalizers, no weak table sweep.
// Safe to call during VM execution (periodic GC from CreateTable).
func (L *State) gcMarkSweep() {
	ls := L.ls()
	g := ls.Global
	if g.GCRunning {
		return
	}
	g.GCRunning = true
	defer func() { g.GCRunning = false }()
	// Clear stale stack slots so traverseThread doesn't mark dead references
	clearStaleStack(ls)
	gc.FullGC(g, ls)
}

// GCCollect runs a full Lua mark-and-sweep GC cycle, then calls all
// pending __gc finalizers. This is the V5 GC entry point.
// Mirrors C Lua's luaC_fullgc + callallpendingfinalizers.
// NOT safe to call during VM execution — use gcMarkSweep for periodic GC.
func (L *State) GCCollect() {
	// Mark as explicit GC — traverseThread uses th.Top (precise marking)
	// so weak tables can collect dead locals in registers above Top.
	g := L.ls().Global
	g.GCExplicit = true
	defer func() { g.GCExplicit = false }()
	// Mark and sweep (with re-entrancy guard)
	L.gcMarkSweep()
	// Call all pending finalizers (objects moved to tobefnz by separateTobeFnz).
	// This runs Lua code via PCall, so must NOT be called during VM execution.
	L.callAllPendingFinalizers()
	// V5 GC handles weak tables natively via clearByValues/clearByKeys.
}

// clearStaleStack nils out stack slots above the highest active frame boundary.
// This ensures the Lua GC doesn't mark stale references left behind when
// Lua locals go out of scope.
// We find the maximum of all CI.Top values and ls.Top, then nil everything above.
func clearStaleStack(ls *state.LuaState) {
	if len(ls.Stack) == 0 {
		return
	}
	// Find the highest stack position used by any active call frame
	maxTop := ls.Top
	for ci := ls.CI; ci != nil; ci = ci.Prev {
		if ci.Top > maxTop {
			maxTop = ci.Top
		}
	}
	// Clear everything above the highest active frame boundary
	for i := maxTop; i < len(ls.Stack); i++ {
		ls.Stack[i].Val = object.Nil
	}
}

// callAllPendingFinalizers drains the tobefnz list, calling each object's
// __gc metamethod via a protected call. Errors are silently discarded.
// Mirrors C Lua's callallpendingfinalizers + GCTM.
func (L *State) callAllPendingFinalizers() {
	ls := L.ls()
	g := ls.Global
	if g.GCRunningFinalizer {
		return // prevent reentrant finalization
	}
	g.GCRunningFinalizer = true
	for g.TobeFnz != nil {
		L.callOneGCTM(ls, g)
	}
	g.GCRunningFinalizer = false
}

// callOneGCTM removes one object from tobefnz, links it back to allgc
// (resurrection), and calls its __gc metamethod via PCall.
// Mirrors C Lua's GCTM.
func (L *State) callOneGCTM(ls *state.LuaState, g *state.GlobalState) {
	// Pop object from tobefnz and link back to allgc
	obj := gc.Udata2Finalize(g)
	if obj == nil {
		return
	}

	// Find the __gc metamethod
	var mt *table.Table
	var objVal object.TValue
	switch v := obj.(type) {
	case *table.Table:
		mt = v.GetMetatable()
		objVal = object.TValue{Obj: v, Tt: object.TagTable}
	case *object.Userdata:
		if v.MetaTable != nil {
			mt, _ = v.MetaTable.(*table.Table)
		}
		objVal = object.TValue{Obj: v, Tt: object.TagUserdata}
	default:
		return
	}
	if mt == nil {
		return
	}
	tmName := g.TMNames[metamethod.TM_GC]
	gcTM := metamethod.GetTM(mt, metamethod.TM_GC, tmName)
	if gcTM.IsNil() {
		return
	}

	// Save absolute stack state. When callOneGCTM runs from periodic GC
	// during VM execution, push/PCall overwrites stack slots above Top
	// that the VM may use as registers. We save those slot values and
	// restore them after PCall returns.
	savedTop := ls.Top
	savedCI := ls.CI
	oldAllowHook := ls.AllowHook

	// Save stack values at the slots we're about to overwrite.
	// push(gcTM) writes to Stack[savedTop], push(objVal) to Stack[savedTop+1].
	// PCall may use additional slots for error handling.
	const saveSlots = 4 // function + arg + potential error + margin
	var savedVals [saveSlots]object.TValue
	for i := 0; i < saveSlots && savedTop+i < len(ls.Stack); i++ {
		savedVals[i] = ls.Stack[savedTop+i].Val
	}

	// Suppress hooks and GC during finalizer (mirrors C Lua's GCTM)
	ls.AllowHook = false

	// Push __gc function and object as argument
	L.push(gcTM)
	L.push(objVal)

	// Mark CI as running a finalizer for debug.getinfo
	ls.CI.CallStatus |= state.CISTFin

	// Protected call: 1 arg, 0 results, no error handler
	L.PCall(1, 0, 0)

	ls.CI.CallStatus &^= state.CISTFin
	ls.AllowHook = oldAllowHook

	// Restore absolute Top, CI, and the original slot values that were
	// overwritten by push/PCall. This prevents corruption of VM
	// registers when callOneGCTM runs during periodic GC.
	ls.CI = savedCI
	ls.Top = savedTop
	for i := 0; i < saveSlots && savedTop+i < len(ls.Stack); i++ {
		ls.Stack[savedTop+i].Val = savedVals[i]
	}
}

// SweepStrings removes dead interned strings from the string table.
func (L *State) SweepStrings() {
	L.strtab().SweepStrings()
}

// gcParamDefaults are the C Lua 5.5.1 default GC parameter values.
// Reference: lgc.h — LUAI_GCPAUSE=200, LUAI_GCMUL=200, LUAI_GCSTEPSIZE=13
var gcParamDefaults = map[string]int64{
	"pause":      200,
	"stepmul":    200,
	"stepsize":   13,
	"minormul":   25,
	"majorminor": 50,
	"minormajor": 100,
}

// GetGCParam returns the current value of a GC parameter.
// For pause/stepmul/stepsize, reads from the direct GlobalState fields.
// For other params, falls back to the GCParams map.
func (L *State) GetGCParam(name string) int64 {
	g := L.ls().Global
	switch name {
	case "pause":
		return int64(g.GCPause)
	case "stepmul":
		return int64(g.GCStepMul)
	case "stepsize":
		return int64(g.GCStepSize)
	}
	if g.GCParams != nil {
		if v, ok := g.GCParams[name]; ok {
			return v
		}
	}
	if def, ok := gcParamDefaults[name]; ok {
		return def
	}
	return 0
}

// SetGCParam sets a GC parameter and returns the previous value.
// For pause/stepmul/stepsize, updates the direct GlobalState fields
// used by the debt-based GC pacer.
func (L *State) SetGCParam(name string, value int64) int64 {
	g := L.ls().Global
	prev := L.GetGCParam(name)
	switch name {
	case "pause":
		g.GCPause = int(value)
	case "stepmul":
		g.GCStepMul = int(value)
	case "stepsize":
		g.GCStepSize = int(value)
	}
	if g.GCParams == nil {
		g.GCParams = make(map[string]int64)
	}
	g.GCParams[name] = value
	return prev
}
