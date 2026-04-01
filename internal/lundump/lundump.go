package lundump

/*
** $Id: lundump.go $
** Load precompiled Lua chunks
** Ported from lundump.h and lundump.c
*/

import (
	"github.com/akzj/go-lua/internal/lobject"
	"github.com/akzj/go-lua/internal/lstate"
	"github.com/akzj/go-lua/internal/lzio"
)

/*
** Load constants
 */
func LoadConstants(L *lstate.LuaState, Z *lzio.ZIO, f *lobject.Proto) error {
	n := int(loadInt(Z))
	f.K = make([]lobject.TValue, n)
	for i := 0; i < n; i++ {
		lobject.SetNilValue(&f.K[i])
	}
	return nil
}

/*
** Load functions
 */
func LoadFunctions(L *lstate.LuaState, Z *lzio.ZIO, f *lobject.Proto) error {
	n := loadInt(Z)
	f.P = make([]*lobject.Proto, n)
	return nil
}

/*
** Load debug info
 */
func LoadDebug(L *lstate.LuaState, Z *lzio.ZIO, f *lobject.Proto) error {
	return nil
}

/*
** Load integer
 */
func loadInt(Z *lzio.ZIO) int64 {
	return 0 // stub
}

/*
** Load size
 */
func loadSize(Z *lzio.ZIO) int64 {
	return int64(0) // stub
}

/*
** Undump - load precompiled chunk
** This is a stub that returns nil for now
 */
func Undump(L *lstate.LuaState, Z *lzio.ZIO, name string) *lobject.LClosure {
	return nil // stub implementation
}
