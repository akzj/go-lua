package lobject

/*
** Basic types and constants
*/

/*
** Basic types
 */
const (
	LUA_TNIL           = 0
	LUA_TBOOLEAN       = 1
	LUA_TLIGHTUSERDATA = 2
	LUA_TNUMBER        = 3
	LUA_TSTRING        = 4
	LUA_TTABLE         = 5
	LUA_TFUNCTION      = 6
	LUA_TUSERDATA      = 7
	LUA_TTHREAD        = 8
	LUA_NUMTYPES       = 9
)

/*
** Extra types for collectable non-values
 */
const (
	LUA_TUPVAL   = LUA_NUMTYPES   // upvalues
	LUA_TPROTO   = LUA_NUMTYPES + 1 // function prototypes
	LUA_TDEADKEY = LUA_NUMTYPES + 2 // removed keys in tables
)

/*
** number of all possible types
 */
const LUA_TOTALTYPES = LUA_TPROTO + 2

/*
** Tagged Value tag bits
 */
const (
	// bits 0-3: actual tag (a LUA_T* constant)
	// bits 4-5: variant bits
	// bit 6: whether value is collectable
)

/*
** Variant tags for nil
 */
const (
	LUA_VNIL     = 0 | (0 << 4) // standard nil
	LUA_VEMPTY   = 0 | (1 << 4) // empty slot
	LUA_VABSTKEY = 0 | (2 << 4) // absent key
	LUA_VNOTABLE = 0 | (3 << 4) // non-table fast access
)

/*
** Variant tags for booleans
 */
const (
	LUA_VFALSE = 1 | (0 << 4)
	LUA_VTRUE  = 1 | (1 << 4)
)

/*
** Variant tags for numbers
 */
const (
	LUA_VNUMINT = 3 | (0 << 4) // integer numbers
	LUA_VNUMFLT = 3 | (1 << 4) // float numbers
)

/*
** Variant tags for strings
 */
const (
	LUA_VSHRSTR = 4 | (0 << 4) | BIT_ISCOLLECTABLE // short strings
	LUA_VLNGSTR = 4 | (1 << 4) | BIT_ISCOLLECTABLE // long strings
)

/*
** Variant tags for table
 */
const LUA_VTABLE = 5 | (0 << 4) | BIT_ISCOLLECTABLE

/*
** Variant tags for threads
 */
const LUA_VTHREAD = 8 | (0 << 4) | BIT_ISCOLLECTABLE

/*
** Variant tags for userdata
 */
const LUA_VLIGHTUSERDATA = 2 | (0 << 4)
const LUA_VUSERDATA = 7 | (0 << 4) | BIT_ISCOLLECTABLE

/*
** Variant tags for functions
 */
const (
	LUA_VLCL = 6 | (0 << 4) | BIT_ISCOLLECTABLE // Lua closure
	LUA_VLCF = 6 | (1 << 4)                   // light C function
	LUA_VCCL = 6 | (2 << 4) | BIT_ISCOLLECTABLE // C closure
)

/*
** Variant tags for upval and proto
 */
const (
	LUA_VUPVAL = 9 | (0 << 4) | BIT_ISCOLLECTABLE
	LUA_VPROTO = 10 | (0 << 4) | BIT_ISCOLLECTABLE
)

/*
** Kinds of long strings
 */
const (
	LSTRREG = -1 // regular long string
	LSTRFIX = -2 // fixed external long string
	LSTRMEM = -3 // external long string with deallocation
)

/*
** Number of reserved words
 */
const NUM_RESERVED = 58

/*
** Tag method names (TM_*)
 */
const (
	TM_INDEX    = 0
	TM_NEWINDEX = 1
	TM_GC       = 2
	TM_MODE     = 3
	TM_LEN      = 4
	TM_EQ       = 5
	TM_ADD      = 6
	TM_SUB      = 7
	TM_MUL      = 8
	TM_MOD      = 9
	TM_POW      = 10
	TM_DIV      = 11
	TM_IDIV     = 12
	TM_BAND     = 13
	TM_BOR      = 14
	TM_BXOR     = 15
	TM_SHL      = 16
	TM_SHR      = 17
	TM_UNM      = 18
	TM_BNOT     = 19
	TM_LT       = 20
	TM_LE       = 21
	TM_CONCAT   = 22
	TM_CALL     = 23
	TM_CLOSE    = 24
	TM_N        = 25
)

/*
** Comparison and arithmetic operators
 */
const (
	LUA_OPADD  = 0
	LUA_OPSUB  = 1
	LUA_OPMUL  = 2
	LUA_OPMOD  = 3
	LUA_OPPOW  = 4
	LUA_OPDIV  = 5
	LUA_OPIDIV = 6
	LUA_OPBAND = 7
	LUA_OPBOR  = 8
	LUA_OPBXOR = 9
	LUA_OPSHL  = 10
	LUA_OPSHR  = 11
	LUA_OPUNM  = 12
	LUA_OPBNOT = 13
	LUA_OPEQ   = 0
	LUA_OPLT   = 1
	LUA_OPLE   = 2
)

/*
** LUA_IDSIZE: maximum size for source description
 */
const LUA_IDSIZE = 60

/*
** MAXSHORTLEN: Maximum length for short strings
 */
const MAXSHORTLEN = 40

/*
** Additional constants for lstate
 */
const (
	LUA_MINSTACK       = 20
	LUA_RIDX_GLOBALS   = 2
	LUA_RIDX_MAINTHREAD = 3
	LUA_RIDX_LAST      = 3
)

// GC constants
const (
	GCSTPGC  = 1
	GCSpause = 0
)

// GC parameters
const (
	GCPAUSE      = 0
	GCMAJORMINOR = 1
	GCMINORMAJOR = 2
	GCSTEPMUL    = 3
	GCSTEPSIZE   = 4
	GENMINORMUL  = 0
	MINORMAJOR   = 1
	MAJORMINOR   = 2
)

// GC parameter values
const (
	LUAI_GCPAUSE      = 200
	LUAI_GCMUL        = 200
	LUAI_GCSTEPSIZE   = 1024
	LUAI_GENMINORMUL  = 20
	LUAI_MINORMAJOR   = 80
	LUAI_MAJORMINOR   = 20
)

// White bits
const (
	WHITE0BIT = 0
	WHITE1BIT = 1
)

// Status codes
const (
	LUA_OK        TStatus = 0
	LUA_YIELD     TStatus = 1
	LUA_ERRRUN    TStatus = 2
	LUA_ERRSYNTAX TStatus = 3
	LUA_ERRMEM    TStatus = 4
	LUA_ERRERR    TStatus = 5
)
