package ltm

/*
** $Id: ltm.go $
** Tag Methods (Metamethods)
** Ported from ltm.h and ltm.c
*/

import (
	"github.com/akzj/go-lua/internal/lobject"
)

/*
** Type names
 */
var TypeNames = [...]string{
	"no value",
	"nil",
	"boolean",
	"userdata",
	"number",
	"string",
	"table",
	"function",
	"userdata",
	"thread",
	"upvalue",
	"proto",
}

/*
** Initialize tag method names
 */
func Init(L *lobject.LuaState) {
	// Initialize tag method names
}

/*
** Get tag method from table
 */
func GetTM(events *lobject.Table, event lobject.TMS, ename *lobject.TString) *lobject.TValue {
	return nil
}

/*
** Get tag method by object
 */
func GetTMByObj(L *lobject.LuaState, o *lobject.TValue, event lobject.TMS) *lobject.TValue {
	return nil
}

/*
** Get object type name
 */
func ObjTypeName(L *lobject.LuaState, o *lobject.TValue) string {
	return lobject.TTypeName(lobject.TType(o))
}
