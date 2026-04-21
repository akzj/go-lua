// Package api defines the tag method (metamethod) system.
//
// Lua supports 25 metamethods that customize object behavior.
// The fasttm optimization caches metamethod absence in the table's flags byte.
//
// Reference: .analysis/07-runtime-infrastructure.md §5
package metamethod

// ---------------------------------------------------------------------------
// TMS is the tag method selector (metamethod index).
// ---------------------------------------------------------------------------
type TMS int

const (
	TM_INDEX    TMS = iota // __index        (0) — fast access
	TM_NEWINDEX            // __newindex     (1) — fast access
	TM_GC                  // __gc           (2) — fast access
	TM_MODE                // __mode         (3) — fast access
	TM_LEN                 // __len          (4) — fast access
	TM_EQ                  // __eq           (5) — fast access (last)
	TM_ADD                 // __add          (6)
	TM_SUB                 // __sub
	TM_MUL                 // __mul
	TM_MOD                 // __mod
	TM_POW                 // __pow
	TM_DIV                 // __div
	TM_IDIV                // __idiv
	TM_BAND                // __band
	TM_BOR                 // __bor
	TM_BXOR                // __bxor
	TM_SHL                 // __shl
	TM_SHR                 // __shr
	TM_UNM                 // __unm          (18)
	TM_BNOT                // __bnot         (19)
	TM_LT                  // __lt           (20)
	TM_LE                  // __le           (21)
	TM_CONCAT              // __concat       (22)
	TM_CALL                // __call         (23)
	TM_CLOSE               // __close        (24)
	TM_N                   // total count = 25
)

// TMNames maps TMS values to their Lua string names.
var TMNames = [TM_N]string{
	"__index", "__newindex", "__gc", "__mode", "__len", "__eq",
	"__add", "__sub", "__mul", "__mod", "__pow", "__div", "__idiv",
	"__band", "__bor", "__bxor", "__shl", "__shr",
	"__unm", "__bnot", "__lt", "__le",
	"__concat", "__call", "__close",
}

// ---------------------------------------------------------------------------
// fasttm cache constants
//
// The first 6 metamethods (TM_INDEX through TM_EQ) are cached in the
// table's flags byte. A set bit means "metamethod is ABSENT".
// ---------------------------------------------------------------------------
const (
	// MaskFlags is the bitmask for fast-access metamethods (bits 0-5).
	MaskFlags byte = 0x3F

	// BitDummy is bit 6 in table flags, indicating the hash part uses dummynode.
	BitDummy byte = 0x40
)

// HasFastTM returns true if the metamethod might be present (cache says not absent).
// mt is the metatable flags byte, event is the TMS index (must be <= TM_EQ).
func HasFastTM(mtFlags byte, event TMS) bool {
	return mtFlags&(1<<uint(event)) == 0
}

// SetAbsent marks a metamethod as absent in the flags cache.
func SetAbsent(flags *byte, event TMS) {
	*flags |= 1 << uint(event)
}

// InvalidateCache clears all fast-access metamethod cache bits.
// Called when a table's metatable is changed.
func InvalidateCache(flags *byte) {
	*flags &^= MaskFlags
}
