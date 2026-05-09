package object

// TableRuntimeError is raised by table operations for Lua runtime errors
// (nil key, NaN key, table overflow). Defined here instead of internal/state
// to avoid an import cycle (state imports table; table cannot import state).
// The VM's runProtected catches this and converts it to a LuaError.
type TableRuntimeError struct{ Msg string }

func (e TableRuntimeError) Error() string { return e.Msg }
