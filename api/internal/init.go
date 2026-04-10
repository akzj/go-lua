// Package internal provides the concrete implementation of api.LuaAPI.
package internal

import (
	luaapi "github.com/akzj/go-lua/api/api"
	memapi "github.com/akzj/go-lua/mem/api"
	tableapi "github.com/akzj/go-lua/table/api"
	types "github.com/akzj/go-lua/types/api"
)

// NewLuaLib creates a new LuaLib implementation.
func NewLuaLib() luaapi.LuaLib {
	return &LuaLibImpl{}
}

func init() {
	// Initialize the default LuaAPI and LuaLib
	luaapi.DefaultLuaAPI = NewLuaState(nil)
	luaapi.DefaultLuaLib = &LuaLibImpl{}
}

// LuaLibImpl is the concrete implementation of luaapi.LuaLib.
type LuaLibImpl struct{}

func (l *LuaLibImpl) NewState() luaapi.LuaAPI {
	return NewLuaState(nil)
}

func (l *LuaLibImpl) NewStateWithAllocator(alloc memapi.Allocator) luaapi.LuaAPI {
	return NewLuaState(alloc)
}

func (l *LuaLibImpl) Register(name string, fn types.CFunction) {}

func (l *LuaLibImpl) LoadString(code string) (luaapi.LuaAPI, error) {
	return nil, nil
}

func (l *LuaLibImpl) DoString(code string) error {
	return nil
}

func (l *LuaLibImpl) LoadBuffer(buff []byte, name, mode string) (luaapi.LuaAPI, error) {
	return nil, nil
}

func (l *LuaLibImpl) DoBuffer(buff []byte, name string) error {
	return nil
}

func (l *LuaLibImpl) CheckInteger(L luaapi.LuaAPI, arg int) int64 {
	return 0
}

func (l *LuaLibImpl) OptInteger(L luaapi.LuaAPI, arg int, def int64) int64 {
	return def
}

func (l *LuaLibImpl) CheckNumber(L luaapi.LuaAPI, arg int) float64 {
	return 0
}

func (l *LuaLibImpl) OptNumber(L luaapi.LuaAPI, arg int, def float64) float64 {
	return def
}

func (l *LuaLibImpl) CheckString(L luaapi.LuaAPI, arg int) string {
	return ""
}

func (l *LuaLibImpl) OptString(L luaapi.LuaAPI, arg int, def string) string {
	return def
}

func (l *LuaLibImpl) CheckAny(L luaapi.LuaAPI, arg int) {}

func (l *LuaLibImpl) CheckType(L luaapi.LuaAPI, arg, t int) {}

func (l *LuaLibImpl) ArgError(L luaapi.LuaAPI, arg int, extraMsg string) int {
	return 0
}

func (l *LuaLibImpl) TypeError(L luaapi.LuaAPI, arg int, tname string) int {
	return 0
}

func (l *LuaLibImpl) NewMetatable(tname string) bool {
	return false
}

func (l *LuaLibImpl) SetMetatableByName(L luaapi.LuaAPI, objIdx int, tname string) {}

func (l *LuaLibImpl) TestUData(L luaapi.LuaAPI, idx int, tname string) interface{} {
	return nil
}

func (l *LuaLibImpl) CheckUData(L luaapi.LuaAPI, idx int, tname string) interface{} {
	return nil
}

func (l *LuaLibImpl) LoadFile(filename string) (luaapi.LuaAPI, error) {
	return nil, nil
}

func (l *LuaLibImpl) DoFile(filename string) error {
	return nil
}

func (l *LuaLibImpl) GSub(s, p, r string) string {
	return s
}

func (l *LuaLibImpl) Where(L luaapi.LuaAPI, level int) {}

func (l *LuaLibImpl) Error(L luaapi.LuaAPI, fmt string, args ...interface{}) int {
	return 0
}

func (l *LuaLibImpl) CallMeta(L luaapi.LuaAPI, obj int, event string) bool {
	return false
}

func (l *LuaLibImpl) Len(L luaapi.LuaAPI, idx int) int64 {
	return 0
}

func (l *LuaLibImpl) GetMetaField(L luaapi.LuaAPI, obj int, e string) bool {
	return false
}

func (l *LuaLibImpl) OpenLibs(L luaapi.LuaAPI) {}

func (l *LuaLibImpl) RequireF(L luaapi.LuaAPI, modname string, openf types.CFunction, glb bool) {}

func (l *LuaLibImpl) NewLib(L luaapi.LuaAPI, regs []luaapi.LuaL_Reg) {}

func (l *LuaLibImpl) SetFuncs(L luaapi.LuaAPI, regs []luaapi.LuaL_Reg, nup int) {}

func (l *LuaLibImpl) NewLibTable(L luaapi.LuaAPI, regs []luaapi.LuaL_Reg) tableapi.TableInterface {
	return nil
}
