# V5 GC Architecture — Self-Managed Mark-and-Sweep in Go

## Overview

V5 replaces Go GC delegation with a self-managed Lua GC. Every collectable Lua
object has a `GCHeader` and lives on a linked list (`allgc`). Lua GC controls
object lifecycle (mark/sweep). Go GC handles memory deallocation after Lua GC
unlinks dead objects.

## GCHeader Design

```go
// In object/api — the leaf package (no import cycles)

type GCObject interface {
    GC() *GCHeader
}

type GCHeader struct {
    Next   GCObject  // allgc/finobj/tobefnz chain
    GCList GCObject  // gray list link
    Marked byte      // GC color + age bits
}
```

Embedded in every collectable type:
- LuaString, Table, LClosure, CClosure, Userdata, LuaState, Proto, UpVal

## GC State Machine

```
GCSpause → GCSpropagate → GCSenteratomic → GCSatomic →
GCSswpallgc → GCSswpfinobj → GCSswptobefnz → GCSswpend → GCScallfin
```

## Sweep = Unlink

```go
func (g *GlobalState) sweepObj(prev, curr GCObject) {
    prev.GC().Next = curr.GC().Next  // unlink from chain
    // Go GC handles deallocation (including cycles)
}
```

No nilling needed — Go's tracing GC handles cycles automatically.

## Write Barriers

~31 call sites. Go compiler inlines barrier functions.
Fast path: check two bit masks → return (3 instructions).

## Object Creation Sites (13 total)

| Type       | Sites | Files                          |
|------------|-------|--------------------------------|
| LuaString  | 2     | luastring/api/api.go           |
| Table      | 3     | table/api, state/api, vm/api   |
| LClosure   | 3     | vm/api (OP_CLOSURE, load, undump) |
| CClosure   | 1     | api/api/impl.go                |
| Userdata   | 1     | api/api/impl.go                |
| LuaState   | 2     | state/api/state.go             |
| Proto      | 4     | vm/api/undump.go, parse/api    |
| UpVal      | 1     | closure/api/closure.go         |

## GlobalState New Fields

- allgc, finobj, tobefnz, fixedgc — object chains
- gcstate, currentwhite — GC phase tracking
- gray, grayagain, weak, allweak, ephemeron — gray lists
- gcdebt, gcestimate, gcstepmul, gcpause — pacing
- survival, old1, reallyold, firstold1 — generational
