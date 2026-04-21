package lua

// This file verifies that our public literal constants stay in sync
// with the internal package constants. These tests run inside the lua
// package (not _test) so they can import internal/.

import (
	"testing"

	"github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/object"
)

func TestConstantsMatchInternal(t *testing.T) {
	// Type values
	checks := []struct {
		name     string
		pub, int int
	}{
		{"TypeNil", int(TypeNil), int(object.TypeNil)},
		{"TypeBoolean", int(TypeBoolean), int(object.TypeBoolean)},
		{"TypeLightUserdata", int(TypeLightUserdata), int(object.TypeLightUserdata)},
		{"TypeNumber", int(TypeNumber), int(object.TypeNumber)},
		{"TypeString", int(TypeString), int(object.TypeString)},
		{"TypeTable", int(TypeTable), int(object.TypeTable)},
		{"TypeFunction", int(TypeFunction), int(object.TypeFunction)},
		{"TypeUserdata", int(TypeUserdata), int(object.TypeUserdata)},
		{"TypeThread", int(TypeThread), int(object.TypeThread)},

		// Pseudo-indices
		{"RegistryIndex", RegistryIndex, api.RegistryIndex},
		{"MultiRet", MultiRet, api.MultiRet},

		// Registry keys
		{"RIdxMainThread", RIdxMainThread, api.RIdxMainThread},
		{"RIdxGlobals", RIdxGlobals, api.RIdxGlobals},

		// Ref
		{"RefNil", RefNil, api.RefNil},
		{"NoRef", NoRef, api.NoRef},

		// Status codes
		{"OK", OK, api.StatusOK},
		{"Yield", Yield, api.StatusYield},
		{"ErrRun", ErrRun, api.StatusErrRun},
		{"ErrSyntax", ErrSyntax, api.StatusErrSyntax},
		{"ErrMem", ErrMem, api.StatusErrMem},
		{"ErrErr", ErrErr, api.StatusErrErr},

		// CompareOp
		{"OpEQ", int(OpEQ), int(api.OpEQ)},
		{"OpLT", int(OpLT), int(api.OpLT)},
		{"OpLE", int(OpLE), int(api.OpLE)},

		// ArithOp
		{"OpAdd", int(OpAdd), int(api.OpAdd)},
		{"OpSub", int(OpSub), int(api.OpSub)},
		{"OpMul", int(OpMul), int(api.OpMul)},
		{"OpMod", int(OpMod), int(api.OpMod)},
		{"OpPow", int(OpPow), int(api.OpPow)},
		{"OpDiv", int(OpDiv), int(api.OpDiv)},
		{"OpIDiv", int(OpIDiv), int(api.OpIDiv)},
		{"OpBAnd", int(OpBAnd), int(api.OpBAnd)},
		{"OpBOr", int(OpBOr), int(api.OpBOr)},
		{"OpBXor", int(OpBXor), int(api.OpBXor)},
		{"OpShl", int(OpShl), int(api.OpShl)},
		{"OpShr", int(OpShr), int(api.OpShr)},
		{"OpUnm", int(OpUnm), int(api.OpUnm)},
		{"OpBNot", int(OpBNot), int(api.OpBNot)},

		// GCWhat
		{"GCStop", int(GCStop), int(api.GCStop)},
		{"GCRestart", int(GCRestart), int(api.GCRestart)},
		{"GCCollect", int(GCCollect), int(api.GCCollect)},
		{"GCCount", int(GCCount), int(api.GCCount)},
		{"GCCountB", int(GCCountB), int(api.GCCountB)},
		{"GCStep", int(GCStep), int(api.GCStep)},
		{"GCIsRunning", int(GCIsRunning), int(api.GCIsRunning)},
		{"GCGen", int(GCGen), int(api.GCGen)},
		{"GCInc", int(GCInc), int(api.GCInc)},
	}
	for _, c := range checks {
		if c.pub != c.int {
			t.Errorf("%s: public=%d, internal=%d", c.name, c.pub, c.int)
		}
	}
}
