// Tag method (metamethod) lookup functions.
//
// Provides GetTM and GetTMByObj for metamethod resolution.
// CallTM and TryBinTM require the VM call machinery and will be
// implemented in Phase 7 (vm).
//
// Reference: lua-master/ltm.c
package metamethod

import (
	objectapi "github.com/akzj/go-lua/internal/object"
	luastringapi "github.com/akzj/go-lua/internal/luastring"
	stateapi "github.com/akzj/go-lua/internal/state"
	tableapi "github.com/akzj/go-lua/internal/table"
)

// ---------------------------------------------------------------------------
// InitTMNames — intern all 25 metamethod name strings.
// Mirrors: luaT_init in ltm.c
//
// NOTE: This is also called from state.NewState via initTMNames.
// This public function is available for use by other modules that need
// to re-initialize or access TM names independently.
// ---------------------------------------------------------------------------

// InitTMNames interns all TM name strings into the global state.
func InitTMNames(g *stateapi.GlobalState, strtab *luastringapi.StringTable) {
	for i := TMS(0); i < TM_N; i++ {
		g.TMNames[i] = strtab.Intern(TMNames[i])
	}
}

// ---------------------------------------------------------------------------
// GetTM — look up a metamethod in a metatable with fasttm cache.
//
// For fast metamethods (TM_INDEX through TM_EQ, indices 0-5), the table's
// Flags byte caches absence. A set bit means "definitely absent".
//
// Mirrors: luaT_gettm in ltm.c
// ---------------------------------------------------------------------------

// GetTM looks up a metamethod in the given metatable.
// Returns the metamethod TValue, or Nil if not found.
// For events <= TM_EQ, uses the fasttm cache on the table's Flags.
func GetTM(mt *tableapi.Table, event TMS, tmName *objectapi.LuaString) objectapi.TValue {
	if mt == nil {
		return objectapi.Nil
	}

	// For fast events (0-5), check the cache first
	if event <= TM_EQ {
		if !mt.HasTagMethod(byte(event)) {
			// Cache says absent — definitely not there
			return objectapi.Nil
		}
	}

	// Look up the metamethod by string key
	val, found := mt.GetStr(tmName)
	if !found || val.IsNil() {
		// Not found — cache absence for fast events
		if event <= TM_EQ {
			mt.SetNoTagMethod(byte(event))
		}
		return objectapi.Nil
	}

	return val
}

// ---------------------------------------------------------------------------
// GetTMByObj — get a metamethod for any Lua value.
//
// For tables: checks the table's own metatable.
// For userdata: checks the userdata's metatable.
// For other types: checks G.MT[type] (global per-type metatable).
//
// Mirrors: luaT_gettmbyobj in ltm.c
// ---------------------------------------------------------------------------

// GetTMByObj looks up a metamethod for the given value.
// Returns the metamethod TValue, or Nil if not found.
func GetTMByObj(g *stateapi.GlobalState, obj objectapi.TValue, event TMS) objectapi.TValue {
	var mt *tableapi.Table

	switch obj.Type() {
	case objectapi.TypeTable:
		// Table: use its own metatable
		if tbl, ok := obj.Val.(*tableapi.Table); ok {
			mt = tbl.GetMetatable()
		}
	case objectapi.TypeUserdata:
		// Userdata: use its metatable
		if ud, ok := obj.Val.(*objectapi.Userdata); ok && ud.MetaTable != nil {
			mt, _ = ud.MetaTable.(*tableapi.Table)
		}
	default:
		// Other types: use global per-type metatable
		typeIdx := int(obj.Type())
		if typeIdx >= 0 && typeIdx < len(g.MT) && g.MT[typeIdx] != nil {
			mt, _ = g.MT[typeIdx].(*tableapi.Table)
		}
	}

	if mt == nil {
		return objectapi.Nil
	}

	// Look up the TM name in the metatable
	tmName := g.TMNames[event]
	if tmName == nil {
		return objectapi.Nil
	}
	val, found := mt.GetStr(tmName)
	if !found {
		return objectapi.Nil
	}
	return val
}

// ---------------------------------------------------------------------------
// ObjTypeName — return the type name for a value, checking __name.
//
// For tables and userdata with a metatable containing __name (a string),
// returns that string. Otherwise returns the standard type name.
//
// Mirrors: luaT_objtypename in ltm.c
// ---------------------------------------------------------------------------

// ObjTypeName returns the type name for a Lua value.
// Checks __name metamethod for tables and userdata.
func ObjTypeName(g *stateapi.GlobalState, obj objectapi.TValue) string {
	var mt *tableapi.Table

	switch obj.Type() {
	case objectapi.TypeTable:
		if tbl, ok := obj.Val.(*tableapi.Table); ok {
			mt = tbl.GetMetatable()
		}
	case objectapi.TypeUserdata:
		if ud, ok := obj.Val.(*objectapi.Userdata); ok && ud.MetaTable != nil {
			mt, _ = ud.MetaTable.(*tableapi.Table)
		}
	}

	if mt != nil {
		// Check for __name field
		strtab, _ := g.StringTable.(*luastringapi.StringTable)
		if strtab != nil {
			nameKey := strtab.Intern("__name")
			if val, found := mt.GetStr(nameKey); found && val.IsString() {
				return val.StringVal().String()
			}
		}
	}

	// Standard type name
	typeIdx := int(obj.Type())
	if typeIdx >= 0 && typeIdx < len(objectapi.TypeNames) {
		return objectapi.TypeNames[typeIdx]
	}
	return "no value"
}
