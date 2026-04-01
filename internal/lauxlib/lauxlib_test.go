package lauxlib

import (
	"testing"

	"github.com/akzj/go-lua/internal/lapi"
	"github.com/akzj/go-lua/internal/lobject"
)

/*
** Test luaL_newstate creates a working Lua state
 */
func TestLuaL_newstate(t *testing.T) {
	L := luaL_newstate()
	if L == nil {
		t.Fatal("luaL_newstate returned nil")
	}
	defer lapi.Lua_close(L)

	// Check basic state
	if L.Status != lobject.LUA_OK {
		t.Errorf("Expected status LUA_OK, got %d", L.Status)
	}

	// Check stack exists
	if len(L.Stack) == 0 {
		t.Error("Stack should be initialized")
	}

	// Check registry table exists via direct access
	registry := &L.G.LRegistry
	if !lobject.TtIsTable(registry) {
		t.Error("Registry table should exist (accessed directly)")
	}
}

/*
** Test luaL_Buffer initialization
 */
func TestLuaL_Buffer(t *testing.T) {
	L := luaL_newstate()
	defer lapi.Lua_close(L)

	var B luaL_Buffer
	luaL_buffinit(L, &B)

	// Check buffer is initialized
	if B.L != L {
		t.Error("Buffer L should be set to L")
	}
	if B.size != LUAL_BUFFERSIZE {
		t.Errorf("Expected size %d, got %d", LUAL_BUFFERSIZE, B.size)
	}
	if B.n != 0 {
		t.Error("Buffer n should be 0 after init")
	}
}

/*
** Test luaL_Buffer add string
 */
func TestLuaL_BufferAdd(t *testing.T) {
	L := luaL_newstate()
	defer lapi.Lua_close(L)

	var B luaL_Buffer
	luaL_buffinit(L, &B)

	// Add a string
	luaL_addstring(&B, "Hello")

	if B.n != 5 {
		t.Errorf("Expected n=5 after addstring, got %d", B.n)
	}

	// Add more string
	luaL_addstring(&B, " World")

	if B.n != 11 {
		t.Errorf("Expected n=11 after second addstring, got %d", B.n)
	}

	// Push result
	luaL_pushresult(&B)

	if lapi.Lua_type(L, -1) != lobject.LUA_TSTRING {
		t.Error("Expected string on stack")
	}

	var length int
	s := lapi.Lua_tolstring(L, -1, &length)
	if s != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", s)
	}
}

/*
** Test luaL_Buffer addvalue
 */
func TestLuaL_BufferAddvalue(t *testing.T) {
	L := luaL_newstate()
	defer lapi.Lua_close(L)

	var B luaL_Buffer
	luaL_buffinit(L, &B)

	// Push a value onto the stack
	lapi.Lua_pushstring(L, "test")

	// Add it to buffer
	luaL_addvalue(&B)

	if B.n != 4 {
		t.Errorf("Expected n=4 after addvalue, got %d", B.n)
	}
}

/*
** Test luaL_Buffer pushresultsize
 */
func TestLuaL_BufferPushresultsize(t *testing.T) {
	L := luaL_newstate()
	defer lapi.Lua_close(L)

	var B luaL_Buffer
	luaL_buffinit(L, &B)

	// Add a string
	luaL_addstring(&B, "Hello")

	// luaL_pushresultsize adds sz to B.n before pushing
	luaL_pushresultsize(&B, 3)

	if lapi.Lua_type(L, -1) != lobject.LUA_TSTRING {
		t.Error("Expected string on stack")
	}
}

/*
** Test buffer with multiple add operations
 */
func TestLuaL_BufferMultiple(t *testing.T) {
	L := luaL_newstate()
	defer lapi.Lua_close(L)

	var B luaL_Buffer
	luaL_buffinit(L, &B)

	// Add multiple strings
	luaL_addstring(&B, "a")
	luaL_addstring(&B, "b")
	luaL_addstring(&B, "c")
	luaL_addstring(&B, "d")
	luaL_addstring(&B, "e")

	luaL_pushresult(&B)

	var length int
	s := lapi.Lua_tolstring(L, -1, &length)
	if s != "abcde" {
		t.Errorf("Expected 'abcde', got '%s'", s)
	}
}

/*
** Test luaL_addchar
 */
func TestLuaL_addchar(t *testing.T) {
	L := luaL_newstate()
	defer lapi.Lua_close(L)

	var B luaL_Buffer
	luaL_buffinit(L, &B)

	luaL_addchar(&B, 'H')
	luaL_addchar(&B, 'i')

	luaL_pushresult(&B)

	var length int
	s := lapi.Lua_tolstring(L, -1, &length)
	if s != "Hi" {
		t.Errorf("Expected 'Hi', got '%s'", s)
	}
}

/*
** Test LUAL_BUFFERSIZE constant
 */
func TestLUAL_BUFFERSIZE(t *testing.T) {
	if LUAL_BUFFERSIZE != 1024 {
		t.Errorf("Expected LUAL_BUFFERSIZE=1024, got %d", LUAL_BUFFERSIZE)
	}
}

/*
** Test makeseed generates a seed
 */
func TestMakeseed(t *testing.T) {
	seed := makeseed()
	// Seed should be non-zero
	if seed == 0 {
		t.Error("Seed should not be 0")
	}
}

/*
** Test registry constants
 */
func TestRegistryConstants(t *testing.T) {
	if LUA_GNAME != "_G" {
		t.Errorf("Expected LUA_GNAME='_G', got '%s'", LUA_GNAME)
	}
	if LUA_LOADED_TABLE != "_LOADED" {
		t.Errorf("Expected LUA_LOADED_TABLE='_LOADED', got '%s'", LUA_LOADED_TABLE)
	}
	if LUA_PRELOAD_TABLE != "_PRELOAD" {
		t.Errorf("Expected LUA_PRELOAD_TABLE='_PRELOAD', got '%s'", LUA_PRELOAD_TABLE)
	}
}

/*
** Test reference constants
 */
func TestReferenceConstants(t *testing.T) {
	if LUA_NOREF != -2 {
		t.Errorf("Expected LUA_NOREF=-2, got %d", LUA_NOREF)
	}
	if LUA_REFNIL != -1 {
		t.Errorf("Expected LUA_REFNIL=-1, got %d", LUA_REFNIL)
	}
}

/*
** Test luaL_checkstack
 */
func TestLuaL_checkstack(t *testing.T) {
	L := luaL_newstate()
	defer lapi.Lua_close(L)

	// Should not panic with valid size
	luaL_checkstack(L, 10, "")
}
