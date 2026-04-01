// Package gc provides Lua 5.5.1 garbage collection.
//
// This package implements mark-sweep GC with generational support.
// Reference: lua-master/lgc.c
package gc

import api "github.com/akzj/go-lua/gc/api"

// Re-export all public types and interfaces
type GCCollector = api.GCCollector
type GCState = api.GCState
type GCBarrier = api.GCBarrier
type GCFixator = api.GCFixator
type GCTrackedObject = api.GCTrackedObject
type GCObjectList = api.GCObjectList
type FinalizableObject = api.FinalizableObject
type GCWeakTable = api.GCWeakTable
type WeakTableMode = api.WeakTableMode

// GC State constants
const (
	GCSpropagate   = api.GCSpropagate
	GCSenteratomic = api.GCSenteratomic
	GCSatomic      = api.GCSatomic
	GCSswpallgc    = api.GCSswpallgc
	GCSswpfinobj   = api.GCSswpfinobj
	GCSswptobefnz  = api.GCSswptobefnz
	GCSswpend      = api.GCSswpend
	GCScallfin     = api.GCScallfin
	GCSpause       = api.GCSpause
)

// Color constants
const (
	White = api.White
	Black = api.Black
)

// Age constants
const (
	GNew       = api.GNew
	GSurvival  = api.GSurvival
	GOld0      = api.GOld0
	GOld1      = api.GOld1
	GOld       = api.GOld
	GTouched1  = api.GTouched1
	GTouched2  = api.GTouched2
)

// GC Kind constants
const (
	KGCInvalid  = api.KGCInvalid
	KGCInc      = api.KGCInc
	KGCGenMinor = api.KGCGenMinor
	KGCGenMajor = api.KGCGenMajor
)

// GC stop constants
const (
	GCstpUser = api.GCstpUser
	GCstpGC   = api.GCstpGC
	GCstpCLS  = api.GCstpCLS
)

// Weak table modes
const (
	WeakKey   = api.WeakKey
	WeakValue = api.WeakValue
)

// Helper functions
var IsWhite        = api.IsWhite
var IsBlack        = api.IsBlack
var IsGray         = api.IsGray
var IsDead         = api.IsDead
var GetAge         = api.GetAge
var SetAge         = api.SetAge
var IsCollectable  = api.IsCollectable
var MakeCollectable = api.MakeCollectable
var Novariant      = api.Novariant
var Ctb            = api.Ctb
var CurrentWhite   = api.CurrentWhite
var OtherWhite     = api.OtherWhite
