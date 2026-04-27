// nodekey_init.go registers the ReconstructObj callback for table node key
// reconstruction. This breaks the import cycle: the table package can't import
// state/closure, but needs to return properly-typed TValues from nodeKey().
package api

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/table"
)

func init() {
	object.ReconstructObj = func(tt object.Tag, ptr unsafe.Pointer) any {
		switch tt {
		case object.TagTable:
			return (*table.Table)(ptr)
		case object.TagUserdata:
			return (*object.Userdata)(ptr)
		case object.TagLuaClosure:
			return (*closure.LClosure)(ptr)
		case object.TagCClosure:
			return (*closure.CClosure)(ptr)
		case object.TagThread:
			return (*state.LuaState)(ptr)
		case object.TagLightCFunc:
			// Light C function: ptr is the data word from the function interface.
			// Reconstruct the function value from the pointer.
			return *(*state.CFunction)(unsafe.Pointer(&ptr))
		default:
			// Unknown type — return nil
			return nil
		}
	}
}
