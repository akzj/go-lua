// gc_impl.go — GC integration (gcMarkSweep, GCCollect, clearStaleStack, finalizers).
package api

import (
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
func (L *State) GCTotalBytes() int64 {
	return L.ls().Global.GCTotalBytes
}

// TrackAlloc adds n bytes to the Lua-level allocation counter.
func (L *State) TrackAlloc(n int64) {
	L.ls().Global.GCTotalBytes += n
}

// SetMemoryLimit sets the maximum memory (in bytes) that Lua objects can use.
// 0 means no limit. Returns the previous limit.
// When the limit is exceeded, TrackAllocation attempts a GC cycle and then
// panics with StatusErrMem if still over limit.
func (L *State) SetMemoryLimit(limit int64) int64 {
	g := L.ls().Global
	prev := g.MemoryLimit
	g.MemoryLimit = limit
	return prev
}

// MemoryLimit returns the current memory limit (0 = no limit).
func (L *State) MemoryLimit() int64 {
	return L.ls().Global.MemoryLimit
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
// Actually switches the GC between incremental and generational modes.
func (L *State) SetGCMode(mode string) string {
	prev := L.GetGCMode()
	ls := L.ls()
	g := ls.Global
	L.ls().Global.GCMode = mode
	switch mode {
	case "generational":
		gc.ChangeMode(g, ls, object.KGC_GENMINOR)
	case "incremental":
		gc.ChangeMode(g, ls, object.KGC_INC)
	}
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
	ls := L.ls()
	g := ls.Global
	g.GCExplicit = true
	defer func() { g.GCExplicit = false }()

	if g.GCKind != object.KGC_INC {
		// Generational mode: full gen collection (minor2inc → entergen)
		if g.GCRunning {
			return
		}
		g.GCRunning = true
		clearStaleStack(ls)
		gc.FullGen(g, ls)
		g.GCRunning = false
		L.callAllPendingFinalizers()
		return
	}

	// Incremental mode: mark and sweep (with re-entrancy guard)
	L.gcMarkSweep()
	// Call all pending finalizers (objects moved to tobefnz by separateTobeFnz).
	// This runs Lua code via PCall, so must NOT be called during VM execution.
	L.callAllPendingFinalizers()
	// V5 GC handles weak tables natively via clearByValues/clearByKeys.
}

// GCStepAPI runs a bounded GC step for collectgarbage("step").
// In incremental mode: runs bounded SingleSteps.
// In generational mode: runs a minor (young) collection.
// Returns true if a full GC cycle completed during this step.
// Mirrors C Lua's lua_gc(LUA_GCSTEP).
func (L *State) GCStepAPI() bool {
	ls := L.ls()
	g := ls.Global
	if g.GCRunning || g.GCRunningFinalizer || g.GCStopped {
		return false
	}
	g.GCRunning = true
	clearStaleStack(ls)

	if g.GCKind != object.KGC_INC {
		// Generational mode: run a minor (young) collection
		gc.YoungCollection(g, ls)
		g.GCRunning = false
		L.callAllPendingFinalizers()
		return false // minor collections don't "complete" a cycle
	}

	// Incremental mode: bounded SingleSteps.
	// GCStep does incremental work and calls SetPause internally if a
	// full cycle completes. We detect completion by checking pause state.
	gc.GCStep(g, ls)
	g.GCRunning = false
	completed := g.GCState == object.GCSpause
	// If cycle completed, drain finalizers
	if completed {
		L.callAllPendingFinalizers()
	}
	return completed
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
	status := L.PCall(1, 0, 0)

	ls.CI.CallStatus &^= state.CISTFin
	ls.AllowHook = oldAllowHook

	// If __gc raised an error, extract error message BEFORE stack restore
	// (the error object is on the stack), but issue the warning AFTER restore
	// so the warn handler can safely push/setglobal without being overwritten.
	var gcErrMsg string
	if status != 0 {
		gcErrMsg = "error object is not a string"
		if ls.Top > 0 {
			errVal := ls.Stack[ls.Top-1].Val
			if errVal.IsString() {
				gcErrMsg = errVal.StringVal().String()
			}
			ls.Top-- // pop error object (mirrors C Lua: L->top.p--)
		}
	}

	// Restore absolute Top, CI, and the original slot values that were
	// overwritten by push/PCall. This prevents corruption of VM
	// registers when callOneGCTM runs during periodic GC.
	ls.CI = savedCI
	ls.Top = savedTop
	for i := 0; i < saveSlots && savedTop+i < len(ls.Stack); i++ {
		ls.Stack[savedTop+i].Val = savedVals[i]
	}

	// Now issue the warning — after stack is restored, so the warn handler
	// (which may call PushString/SetGlobal for @store mode) operates safely.
	if status != 0 {
		g.WarnError("__gc", gcErrMsg)
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

// ---------------------------------------------------------------------------
// GC Inspection API — used by T library (testlib.go)
// ---------------------------------------------------------------------------

// GCObjectAt returns the GCHeader for the GC-collectable object at stack index idx.
// Returns nil if the value is not a GC object (nil, boolean, number, light userdata).
func (L *State) GCObjectAt(idx int) *object.GCHeader {
	v := L.index2val(idx)
	if v == nil {
		return nil
	}
	if gcObj, ok := v.Obj.(object.GCObject); ok && gcObj != nil {
		return gcObj.GC()
	}
	return nil
}

// GCStateName returns the name of the current GC state.
// Maps GCState byte constants to their C Lua names.
func (L *State) GCStateName() string {
	g := L.ls().Global
	return gcStateToName(g.GCState)
}

// RunGCUntilState advances the GC state machine by calling SingleStep
// until it reaches the target state. Returns the state name reached.
// Used by T.gcstate("statename") in the test library.
func (L *State) RunGCUntilState(targetState byte) string {
	ls := L.ls()
	g := ls.Global
	// Clear stale stack first (same as GCCollect)
	clearStaleStack(ls)
	// Run SingleStep until we reach the target state.
	// Safety limit to prevent infinite loops.
	for i := 0; i < 100000; i++ {
		if g.GCState == targetState {
			return gcStateToName(g.GCState)
		}
		gc.SingleStep(g, ls)
	}
	return gcStateToName(g.GCState)
}

// gcStateToName converts a GC state byte to its C Lua name.
func gcStateToName(st byte) string {
	switch st {
	case object.GCSpause:
		return "pause"
	case object.GCSpropagate:
		return "propagate"
	case object.GCSenteratomic:
		return "enteratomic"
	case object.GCSatomic:
		return "atomic"
	case object.GCSswpallgc:
		return "sweepallgc"
	case object.GCSswpfinobj:
		return "sweepfinobj"
	case object.GCSswptobefnz:
		return "sweeptobefnz"
	case object.GCSswpend:
		return "sweepend"
	case object.GCScallfin:
		return "callfin"
	default:
		return "unknown"
	}
}

// gcNameToState converts a C Lua state name to its byte constant.
// Returns (state, ok). If the name is not recognized, ok is false.
func gcNameToState(name string) (byte, bool) {
	switch name {
	case "pause":
		return object.GCSpause, true
	case "propagate":
		return object.GCSpropagate, true
	case "enteratomic":
		return object.GCSenteratomic, true
	case "atomic":
		return object.GCSatomic, true
	case "sweepallgc":
		return object.GCSswpallgc, true
	case "sweepfinobj":
		return object.GCSswpfinobj, true
	case "sweeptobefnz":
		return object.GCSswptobefnz, true
	case "sweepend":
		return object.GCSswpend, true
	case "callfin":
		return object.GCScallfin, true
	default:
		return 0, false
	}
}

// GCColorName returns "white", "gray", or "black" for the GC object at idx.
// Returns "" if the value is not a GC object.
func (L *State) GCColorName(idx int) string {
	h := L.GCObjectAt(idx)
	if h == nil {
		return ""
	}
	if h.IsBlack() {
		return "black"
	}
	if h.IsWhite() {
		return "white"
	}
	return "gray"
}

// GCAgeName returns the generational age name for the GC object at idx.
// Returns "" if the value is not a GC object.
func (L *State) GCAgeName(idx int) string {
	h := L.GCObjectAt(idx)
	if h == nil {
		return ""
	}
	switch h.Age {
	case object.G_NEW:
		return "new"
	case object.G_SURVIVAL:
		return "survival"
	case object.G_OLD0:
		return "old0"
	case object.G_OLD1:
		return "old1"
	case object.G_OLD:
		return "old"
	case object.G_TOUCHED1:
		return "touched1"
	case object.G_TOUCHED2:
		return "touched2"
	default:
		return "new"
	}
}

// TableSizes returns the array part length and hash part capacity for a table at idx.
// Returns (0, 0) if the value is not a table.
func (L *State) TableSizes(idx int) (int, int) {
	v := L.index2val(idx)
	if v == nil {
		return 0, 0
	}
	t, ok := v.Obj.(*table.Table)
	if !ok || t == nil {
		return 0, 0
	}
	return len(t.Array), len(t.Nodes)
}
