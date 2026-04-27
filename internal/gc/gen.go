// gen.go — Generational GC (minor/major collections).
// Mirrors C Lua's lgc.c generational mode (lgc.c:1131-1480).
package gc

import (
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/state"
)

// ---------------------------------------------------------------------------
// InitGenMode — lightweight initialization of generational GC mode.
// Used by NewState to set up gen mode without running a full GC cycle.
// Marks all existing objects as OLD and sets gen boundaries so that
// new allocations will be young (collected cheaply by YoungCollection).
// ---------------------------------------------------------------------------

func InitGenMode(g *state.GlobalState) {
	// Mark all existing objects as OLD
	for obj := g.Allgc; obj != nil; obj = obj.GC().Next {
		obj.GC().Age = object.G_OLD
	}
	for obj := g.FinObj; obj != nil; obj = obj.GC().Next {
		obj.GC().Age = object.G_OLD
	}
	for obj := g.TobeFnz; obj != nil; obj = obj.GC().Next {
		obj.GC().Age = object.G_OLD
	}

	// Set gen mode state
	g.GCKind = object.KGC_GENMINOR
	g.GCState = object.GCSpropagate

	// Set gen boundaries: everything is old
	g.Survival = g.Allgc
	g.Old1 = g.Allgc
	g.ReallyOld = g.Allgc
	g.FirstOld1 = nil
	g.FinObjSur = g.FinObj
	g.FinObjOld1 = g.FinObj
	g.FinObjROld = g.FinObj

	// Set baseline for minor→major promotion
	g.GCMajorMinor = g.GCTotalBytes
	g.GCMarked = 0
}

// ---------------------------------------------------------------------------
// sweep2old — sweep list, mark all alive objects as OLD.
// Used when entering gen mode: everything alive becomes old.
// Dead objects (other-white) are unlinked. Threads go to grayagain.
// Mirrors C Lua's sweep2old (lgc.c:1136).
// ---------------------------------------------------------------------------

func sweep2old(g *state.GlobalState, p *object.GCObject) {
	otherwhite := otherWhite(g)
	for *p != nil {
		obj := *p
		h := obj.GC()
		if isDeadMark(otherwhite, h.Marked) {
			// Dead — unlink
			*p = h.Next
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
		} else {
			// Alive — make old
			h.Age = object.G_OLD
			if _, ok := obj.(*state.LuaState); ok {
				// Threads must be watched — put on grayagain
				h.Marked &^= object.BlackBit // make gray
				g.GrayAgain = append(g.GrayAgain, obj)
			} else {
				// Everything else becomes black
				h.Marked = (h.Marked &^ object.WhiteBits) | object.BlackBit
			}
			p = &h.Next
		}
	}
}

// ---------------------------------------------------------------------------
// sweepgen — generational sweep. Advances ages and removes dead objects.
// Returns pointer to the last live element's Next field (boundary pointer).
// pfirstold1: set to first OLD1 object found (for optimization).
// paddedold: accumulates bytes of objects that became old.
// Mirrors C Lua's sweepgen (lgc.c:1176).
// ---------------------------------------------------------------------------

// nextAge maps current age → next age after a minor collection.
var nextAge = [7]byte{
	object.G_SURVIVAL, // from G_NEW
	object.G_OLD1,     // from G_SURVIVAL
	object.G_OLD1,     // from G_OLD0
	object.G_OLD,      // from G_OLD1
	object.G_OLD,      // from G_OLD (no change)
	object.G_TOUCHED1, // from G_TOUCHED1 (no change)
	object.G_TOUCHED2, // from G_TOUCHED2 (no change)
}

func sweepgen(g *state.GlobalState, p *object.GCObject, limit object.GCObject,
	pfirstold1 *object.GCObject, paddedold *int64) *object.GCObject {

	var addedold int64
	currentWhite := g.CurrentWhite

	for *p != limit {
		obj := *p
		if obj == nil {
			break
		}
		h := obj.GC()

		otherwhite := otherWhite(g)
		if isDeadMark(otherwhite, h.Marked) {
			// Dead — unlink
			*p = h.Next
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
		} else {
			// Alive — correct mark and age
			age := h.Age
			if age == object.G_NEW {
				// New objects go back to white (survival)
				h.Marked = (h.Marked &^ (object.WhiteBits | object.BlackBit)) | currentWhite
				h.Age = object.G_SURVIVAL
			} else {
				// All other objects advance age, keep their color
				h.Age = nextAge[age]
				if h.Age == object.G_OLD1 {
					addedold += h.ObjSize
					if *pfirstold1 == nil {
						*pfirstold1 = obj
					}
				}
			}
			p = &h.Next
		}
	}
	*paddedold += addedold
	return p
}

// ---------------------------------------------------------------------------
// correctgraylist — correct a gray list after generational sweep.
// TOUCHED1 → TOUCHED2 (remain on list). Threads remain on list.
// TOUCHED2 → OLD (removed). White objects removed.
// Returns pointer to the tail of the corrected list.
// Mirrors C Lua's correctgraylist (lgc.c:1226).
// ---------------------------------------------------------------------------

func correctgraylist(list *[]object.GCObject) {
	n := 0
	for _, obj := range *list {
		h := obj.GC()
		if h.IsWhite() {
			// Remove white objects
			continue
		}
		if h.Age == object.G_TOUCHED1 {
			// Touched in this cycle → advance to TOUCHED2, make black, keep
			h.Marked = (h.Marked &^ object.WhiteBits) | object.BlackBit
			h.Age = object.G_TOUCHED2
			(*list)[n] = obj
			n++
			continue
		}
		if _, ok := obj.(*state.LuaState); ok {
			// Non-white threads stay on the list
			(*list)[n] = obj
			n++
			continue
		}
		// Everything else removed (TOUCHED2 → OLD, etc.)
		if h.Age == object.G_TOUCHED2 {
			h.Age = object.G_OLD
		}
		h.Marked = (h.Marked &^ object.WhiteBits) | object.BlackBit
	}
	*list = (*list)[:n]
}

// correctgraylists corrects all gray lists, merging them into grayagain.
// Mirrors C Lua's correctgraylists (lgc.c:1260).
func correctgraylists(g *state.GlobalState) {
	// Merge weak/allweak/ephemeron into grayagain, then correct all at once
	g.GrayAgain = append(g.GrayAgain, g.Weak...)
	g.Weak = g.Weak[:0]
	g.GrayAgain = append(g.GrayAgain, g.AllWeak...)
	g.AllWeak = g.AllWeak[:0]
	g.GrayAgain = append(g.GrayAgain, g.Ephemeron...)
	g.Ephemeron = g.Ephemeron[:0]
	correctgraylist(&g.GrayAgain)
}

// ---------------------------------------------------------------------------
// markold — mark OLD1 objects to be re-traversed in minor collection.
// Walks the allgc chain from 'from' to 'to', marking OLD1 objects.
// Mirrors C Lua's markold (lgc.c:1282).
// ---------------------------------------------------------------------------

func markold(g *state.GlobalState, from, to object.GCObject) {
	for p := from; p != to; p = p.GC().Next {
		if p == nil {
			break
		}
		h := p.GC()
		if h.Age == object.G_OLD1 {
			h.Age = object.G_OLD // now they are old
			if h.IsBlack() {
				markObject(g, p) // re-mark for traversal
			}
		}
	}
}

// ---------------------------------------------------------------------------
// finishgencycle — finish a young-generation collection.
// Mirrors C Lua's finishgencycle (lgc.c:1296).
// ---------------------------------------------------------------------------

func finishgencycle(g *state.GlobalState, L *state.LuaState) {
	correctgraylists(g)
	g.GCState = object.GCSpropagate // skip restart
}

// ---------------------------------------------------------------------------
// checkminormajor — check whether minor collection should shift to major.
// Returns true if bytes becoming old exceed GCMinorMajor% of the baseline
// (GCMajorMinor = bytes marked in last major collection).
// Mirrors C Lua's checkminormajor (lgc.c:1323).
// ---------------------------------------------------------------------------

func checkminormajor(g *state.GlobalState) bool {
	if g.GCMinorMajor == 0 {
		return false // special case: 0 disables major promotion
	}
	limit := g.GCMajorMinor * int64(g.GCMinorMajor) / 100
	return g.GCMarked >= limit
}

// ---------------------------------------------------------------------------
// SetMinorDebt — set GC debt after a minor (young) collection.
// debt = GCMajorMinor * GCMinorMul / 100
// Mirrors C Lua's setminordebt (lgc.c:1417).
// ---------------------------------------------------------------------------

func SetMinorDebt(g *state.GlobalState) {
	debt := g.GCMajorMinor * int64(g.GCMinorMul) / 100
	const minDebt int64 = 64 * 1024 // minimum to avoid thrashing
	if debt < minDebt {
		debt = minDebt
	}
	g.GCDebt = debt
}

// ---------------------------------------------------------------------------
// youngcollection — the core minor (young) collection.
// Only scans new/survival objects. Old objects are skipped.
// Mirrors C Lua's youngcollection (lgc.c:1335).
// ---------------------------------------------------------------------------

func YoungCollection(g *state.GlobalState, L *state.LuaState) {
	var addedold1 int64
	var dummy object.GCObject // dummy for finobj (no firstold1 optimization)

	// Flip white for this collection cycle. Objects created between cycles
	// have the OLD white. After flip, they become "other" white. Unreachable
	// ones stay white → dead. Reachable ones get marked black → alive.
	g.CurrentWhite ^= object.WhiteBits

	// 1. Mark OLD1 objects (they need re-traversal)
	if g.FirstOld1 != nil {
		markold(g, g.FirstOld1, g.ReallyOld)
		g.FirstOld1 = nil
	}
	markold(g, g.FinObj, g.FinObjROld)
	markold(g, g.TobeFnz, nil)

	// 2. Run atomic phase (mark + propagate everything reachable)
	atomicPhase(g, L)

	// 3. Sweep nursery (new objects) → get pointer to last live element
	g.GCState = object.GCSswpallgc
	psurvival := sweepgen(g, &g.Allgc, g.Survival, &g.FirstOld1, &addedold1)
	// Sweep survival objects
	sweepgen(g, psurvival, g.Old1, &g.FirstOld1, &addedold1)
	g.ReallyOld = g.Old1
	g.Old1 = *psurvival // survival survivors are old now
	g.Survival = g.Allgc // all new are survivals now

	// 4. Repeat for finobj lists
	psurvival = sweepgen(g, &g.FinObj, g.FinObjSur, &dummy, &addedold1)
	sweepgen(g, psurvival, g.FinObjOld1, &dummy, &addedold1)
	g.FinObjROld = g.FinObjOld1
	g.FinObjOld1 = *psurvival
	g.FinObjSur = g.FinObj

	// 5. Sweep tobefnz
	sweepgen(g, &g.TobeFnz, nil, &dummy, &addedold1)

	// 6. Track bytes becoming old (for minor→major promotion check)
	g.GCMarked += addedold1

	// 7. Decide whether to shift to major mode
	if checkminormajor(g) {
		minor2inc(g, L, object.KGC_GENMAJOR) // go to major mode
		g.GCMarked = 0                        // avoid pause in first major cycle
	} else {
		finishgencycle(g, L) // still in minor mode
	}
}

// ---------------------------------------------------------------------------
// cleargraylists — clear all gray lists (for entering gen mode).
// ---------------------------------------------------------------------------

func cleargraylists(g *state.GlobalState) {
	g.Gray = g.Gray[:0]
	g.GrayAgain = g.GrayAgain[:0]
	g.Weak = g.Weak[:0]
	g.AllWeak = g.AllWeak[:0]
	g.Ephemeron = g.Ephemeron[:0]
}

// ---------------------------------------------------------------------------
// atomic2gen — transition from incremental to generational mode.
// Clears gray lists, sweeps all objects to OLD, sets up gen boundaries.
// Mirrors C Lua's atomic2gen (lgc.c:1388).
// ---------------------------------------------------------------------------

func atomic2gen(g *state.GlobalState, L *state.LuaState) {
	cleargraylists(g)

	// Sweep all elements making them old
	g.GCState = object.GCSswpallgc
	sweep2old(g, &g.Allgc)

	// Everything alive now is old
	g.ReallyOld = g.Allgc
	g.Old1 = g.Allgc
	g.Survival = g.Allgc
	g.FirstOld1 = nil

	// Repeat for finobj
	sweep2old(g, &g.FinObj)
	g.FinObjROld = g.FinObj
	g.FinObjOld1 = g.FinObj
	g.FinObjSur = g.FinObj

	// Sweep tobefnz
	sweep2old(g, &g.TobeFnz)

	g.GCKind = object.KGC_GENMINOR
	g.GCState = object.GCSpropagate // skip restart

	// Set baseline for minor→major promotion: total bytes after major collection.
	// Mirrors C Lua's g->GCmajorminor = g->GCmarked (lgc.c:1405).
	g.GCMajorMinor = g.GCTotalBytes
	g.GCMarked = 0 // reset accumulated old bytes

	finishgencycle(g, L)
}

// ---------------------------------------------------------------------------
// entergen — enter generational mode. Runs a full incremental cycle
// through the atomic phase, then transitions to gen mode.
// Mirrors C Lua's entergen (lgc.c:1430).
// ---------------------------------------------------------------------------

func EnterGen(g *state.GlobalState, L *state.LuaState) {
	// Run until pause (prepare to start a new cycle)
	runTilState(g, L, object.GCSpause)
	// Start new cycle (pause → propagate)
	runTilState(g, L, object.GCSpropagate)
	// Run atomic phase
	atomicPhase(g, L)
	// Transition to gen mode
	atomic2gen(g, L)
}

// runTilState runs SingleStep until the GC reaches the target state.
// If allowPastTarget is true, it also stops if the state goes past target.
func runTilState(g *state.GlobalState, L *state.LuaState, target byte) {
	for g.GCState != target {
		SingleStep(g, L)
	}
}

// ---------------------------------------------------------------------------
// minor2inc — shift from minor (gen) collection back to incremental.
// Clears gen boundaries and enters sweep phase.
// Mirrors C Lua's minor2inc (lgc.c:1310).
// ---------------------------------------------------------------------------

func minor2inc(g *state.GlobalState, L *state.LuaState, kind byte) {
	g.GCKind = kind
	// Save baseline for next gen cycle (matches C Lua: g->GCmajorminor = g->GCmarked)
	g.GCMajorMinor = g.GCTotalBytes
	g.Survival = nil
	g.Old1 = nil
	g.ReallyOld = nil
	g.FirstOld1 = nil
	g.FinObjSur = nil
	g.FinObjOld1 = nil
	g.FinObjROld = nil
	// Reset all ages to G_NEW
	for obj := g.Allgc; obj != nil; obj = obj.GC().Next {
		obj.GC().Age = object.G_NEW
	}
	for obj := g.FinObj; obj != nil; obj = obj.GC().Next {
		obj.GC().Age = object.G_NEW
	}
	for obj := g.TobeFnz; obj != nil; obj = obj.GC().Next {
		obj.GC().Age = object.G_NEW
	}
	entersweep(g)
}

// ---------------------------------------------------------------------------
// FullGen — full collection in generational mode.
// Switches to incremental, does a full cycle, then re-enters gen mode.
// Mirrors C Lua's fullgen (lgc.c:1455).
// ---------------------------------------------------------------------------

func FullGen(g *state.GlobalState, L *state.LuaState) {
	minor2inc(g, L, object.KGC_INC)
	EnterGen(g, L)
}

// ---------------------------------------------------------------------------
// ChangeMode — switch between incremental and generational GC modes.
// Mirrors C Lua's luaC_changemode (lgc.c:1440).
// ---------------------------------------------------------------------------

func ChangeMode(g *state.GlobalState, L *state.LuaState, newmode byte) {
	if g.GCKind == object.KGC_GENMAJOR {
		g.GCKind = object.KGC_INC // already incremental in name
	}
	if newmode != g.GCKind {
		if newmode == object.KGC_INC {
			minor2inc(g, L, object.KGC_INC)
		} else {
			// newmode == KGC_GENMINOR
			EnterGen(g, L)
		}
	}
}
