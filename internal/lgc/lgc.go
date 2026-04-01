package lgc

/*
** $Id: lgc.go $
** Garbage Collector
** Ported from lgc.h and lgc.c
*/

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** GC states
 */
const (
	GCSpropagate   = 0
	GCSenteratomic = 1
	GCSatomic      = 2
	GCSswpallgc    = 3
	GCSswpfinobj   = 4
	GCSswptobefnz  = 5
	GCSswpend      = 6
	GCScallfin     = 7
	GCSpause       = 8
)

/*
** Maximum number of elements to sweep in each single step.
 */
const GCSWEEPMAX = 20

/*
** Cost (in work units) of running one finalizer.
 */
const CWUFIN = 10

/*
** Color bit masks
 */
const (
	WHITE0BIT    = 3
	WHITE1BIT    = 4
	BLACKBIT     = 5
	FINALIZEDBIT = 6
	TESTBIT      = 7
)

func bitmask(b int) lobject.LuByte {
	return 1 << uint(b)
}

func bit2mask(b1, b2 int) lobject.LuByte {
	return bitmask(b1) | bitmask(b2)
}

// WHITEBITS is calculated at runtime
var WHITEBITS = bit2mask(WHITE0BIT, WHITE1BIT)

/*
** Object colors
 */
func iswhite(x *lobject.GCObject) bool {
	return (x.Marked & WHITEBITS) != 0
}

func isblack(x *lobject.GCObject) bool {
	return (x.Marked & bitmask(BLACKBIT)) != 0
}

func isgray(x *lobject.GCObject) bool {
	return !iswhite(x) && !isblack(x)
}

func tofinalize(x *lobject.GCObject) bool {
	return (x.Marked & bitmask(FINALIZEDBIT)) != 0
}

func otherwhite(g *lstate.GlobalState) lobject.LuByte {
	return g.CurrentWhite ^ WHITEBITS
}

func isdeadm(ow, m lobject.LuByte) bool {
	return (m & ow) != 0
}

func isdead(g *lstate.GlobalState, v *lobject.GCObject) bool {
	return isdeadm(otherwhite(g), v.Marked)
}

func changewhite(x *lobject.GCObject) {
	x.Marked ^= WHITEBITS
}

func nw2black(x *lobject.GCObject) {
	x.Marked |= bitmask(BLACKBIT)
}

func luaC_white(g *lstate.GlobalState) lobject.LuByte {
	return g.CurrentWhite & WHITEBITS
}

/*
** Make an object white (reset its color)
 */
func makewhite(g *lstate.GlobalState, x *lobject.GCObject) {
	x.Marked = (x.Marked &^ WHITEBITS) | luaC_white(g)
}

/*
** Make an object gray (neither white nor black)
 */
func set2gray(x *lobject.GCObject) {
	x.Marked &^= WHITEBITS
}

/*
** Make an object black
 */
func set2black(x *lobject.GCObject) {
	x.Marked = (x.Marked &^ WHITEBITS) | bitmask(BLACKBIT)
}

/*
** Check if value is collectable and white
 */
func valiswhite(o *lobject.TValue) bool {
	return lobject.IsCollectable(o) && iswhite(lobject.GcValue(o))
}

/*
** Global state access
 */
func G(L *lstate.LuaState) *lstate.GlobalState {
	return L.G
}

/*
** Barrier that moves collector forward
 */
func luaC_barrier_(L *lstate.LuaState, o, v *lobject.GCObject) {
	g := G(L)
	if isblack(o) && iswhite(v) && !isdead(g, v) && !isdead(g, o) {
		reallymarkobject(g, v)
	}
}

/*
** Barrier that moves collector backward
 */
func luaC_barrierback_(L *lstate.LuaState, o *lobject.GCObject) {
	g := G(L)
	if isblack(o) && !isdead(g, o) {
		linkobjgclist(o, &g.GrayAgain)
	}
}

/*
** Mark an object.
 */
func reallymarkobject(g *lstate.GlobalState, o *lobject.GCObject) {
	switch o.Tt {
	case lobject.LUA_VSHRSTR, lobject.LUA_VLNGSTR:
		set2black(o)
	case lobject.LUA_VTABLE:
		linkobjgclist(o, &g.Gray)
	case lobject.LUA_VLCL, lobject.LUA_VCCL:
		linkobjgclist(o, &g.Gray)
	case lobject.LUA_VTHREAD:
		linkobjgclist(o, &g.Gray)
	case lobject.LUA_VPROTO:
		linkobjgclist(o, &g.Gray)
	case lobject.LUA_VUSERDATA:
		linkobjgclist(o, &g.Gray)
	default:
		// Unknown type
	}
}

func linkobjgclist(o *lobject.GCObject, list **lobject.GCObject) {
	o.Next = *list
	*list = o
	set2gray(o)
}

/*
** Mark value
 */
func markvalue(g *lstate.GlobalState, o *lobject.TValue) {
	if valiswhite(o) {
		reallymarkobject(g, lobject.GcValue(o))
	}
}

/*
** Mark object
 */
func markobject(g *lstate.GlobalState, t *lobject.GCObject) {
	if iswhite(t) {
		reallymarkobject(g, t)
	}
}

/*
** Create a new collectable object
 */
func luaC_newobjdt(L *lstate.LuaState, tt lobject.LuByte, sz uint64, offset uint64) *lobject.GCObject {
	g := G(L)
	// Allocate memory (simplified)
	o := &lobject.GCObject{
		Tt:     tt,
		Marked: luaC_white(g),
		Next:   g.Allgc,
	}
	g.Allgc = o
	return o
}

/*
** Create a new collectable object with no offset.
 */
func luaC_newobj(L *lstate.LuaState, tt lobject.LuByte, sz uint64) *lobject.GCObject {
	return luaC_newobjdt(L, tt, sz, 0)
}

/*
** Fix an object (will never be collected)
 */
func luaC_fix(L *lstate.LuaState, o *lobject.GCObject) {
	g := G(L)
	set2gray(o)
	g.Allgc = o.Next
	o.Next = g.Fixedgc
	g.Fixedgc = o
}

/*
** Free all objects
 */
func luaC_freeallobjects(L *lstate.LuaState) {
	g := G(L)
	g.GCState = GCSpause
	g.CurrentWhite = bitmask(WHITE0BIT)

	// Free allgc list
	for o := g.Allgc; o != nil; o = o.Next {
		// Free object
	}
	g.Allgc = nil
}

/*
** Do one step of garbage collection
 */
func luaC_step(L *lstate.LuaState) {
	g := G(L)

	switch g.GCState {
	case GCSpause:
		restartcollection(g)
		g.GCState = GCSpropagate

	case GCSpropagate:
		if g.Gray != nil {
			// Traverse gray objects
			propagatemark(g)
		} else {
			g.GCState = GCSenteratomic
		}

	case GCSenteratomic:
		atomic_(L)
		g.GCState = GCSswpallgc

	case GCSswpallgc:
		sweep(L, &g.Allgc)
		g.GCState = GCSswpfinobj

	case GCSswpfinobj:
		sweep(L, &g.Finobj)
		g.GCState = GCScallfin

	case GCScallfin:
		g.GCState = GCSswpend

	case GCSswpend:
		g.GCState = GCSpause
	}
}

/*
** Restart collection
 */
func restartcollection(g *lstate.GlobalState) {
	g.Gray = nil
	g.GrayAgain = nil
	g.Weak = nil
	g.Ephemeron = nil
	g.Allweak = nil
	mainTh := lstate.MainThreadPtr(g)
	markobject(g, (*lobject.GCObject)(unsafe.Pointer(mainTh)))
}

/*
** Propagate gray marking
 */
func propagatemark(g *lstate.GlobalState) {
	o := g.Gray
	if o == nil {
		return
	}
	g.Gray = o.Next
	set2black(o)
}

/*
** Atomic phase
 */
func atomic_(L *lstate.LuaState) {
	g := G(L)
	// Process gray again list
	for g.GrayAgain != nil {
		o := g.GrayAgain
		g.GrayAgain = o.Next
		set2black(o)
	}
}

/*
** Sweep a list of objects
 */
func sweep(L *lstate.LuaState, list **lobject.GCObject) {
	// Simplified sweep implementation
}

/*
** Full garbage collection
 */
func luaC_fullgc(L *lstate.LuaState, isemergency int) {
	g := G(L)

	if g.GCState == GCSpause {
		restartcollection(g)
		g.GCState = GCSpropagate
	}

	for g.GCState != GCSpause {
		luaC_step(L)
	}
}

/*
** GC barrier for assignments
 */
func luaC_objbarrier(L *lstate.LuaState, p, o *lobject.GCObject) {
	if isblack(p) && iswhite(o) {
		luaC_barrier_(L, p, o)
	}
}

/*
** GC barrier for values
 */
func luaC_barrier(L *lstate.LuaState, p *lobject.GCObject, v *lobject.TValue) {
	if lobject.IsCollectable(v) {
		luaC_objbarrier(L, p, lobject.GcValue(v))
	}
}

/*
** Back barrier
 */
func luaC_objbarrierback(L *lstate.LuaState, p, o *lobject.GCObject) {
	if isblack(p) && iswhite(o) {
		luaC_barrierback_(L, p)
	}
}

/*
** Back barrier for values
 */
func luaC_barrierback(L *lstate.LuaState, p *lobject.GCObject, v *lobject.TValue) {
	if lobject.IsCollectable(v) {
		luaC_objbarrierback(L, p, lobject.GcValue(v))
	}
}

/*
** Fast barrier for table set
 */
func BarrierBack(L *lstate.LuaState, gco *lobject.GCObject) {
	g := G(L)
	if isblack(gco) && !isdead(g, gco) {
		linkobjgclist(gco, &g.GrayAgain)
	}
}

/*
** Run until GC reaches a certain state
 */
func luaC_runtilstate(L *lstate.LuaState, state int, fast int) {
	g := G(L)
	for g.GCState != uint8(state) {
		luaC_step(L)
	}
}