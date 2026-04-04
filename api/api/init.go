// Package api provides Lua C API compatible interfaces.
package api

// Note: Due to import cycle (api/api defines LuaAPI interface, 
// api/internal implements it), DefaultLuaAPI cannot be auto-initialized.
// Use api/internal.NewLuaState(nil) directly to create a state.
