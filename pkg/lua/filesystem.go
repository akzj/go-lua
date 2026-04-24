package lua

import "io/fs"

// SetFileSystem sets a custom [fs.FS] for Lua file operations.
// When set, [State.LoadFile], [State.DoFile], and the package.searchers
// Lua-file searcher will read from this FS instead of the real filesystem.
//
// This enables loading Lua scripts from Go's embed.FS, in-memory
// filesystems, or any other [fs.FS] implementation:
//
//	//go:embed lua/*
//	var luaFS embed.FS
//	sub, _ := fs.Sub(luaFS, "lua")
//	L.SetFileSystem(sub)
//	L.DoString(`require("mymodule")`)  // loads from embedded FS
//
// Set to nil to revert to the real OS filesystem (default).
func (L *State) SetFileSystem(fsys fs.FS) {
	L.fileSystem = fsys
	L.s.FileSystem = fsys // propagate to internal api.State
}

// FileSystem returns the current filesystem, or nil for the real OS filesystem.
func (L *State) FileSystem() fs.FS {
	return L.fileSystem
}
