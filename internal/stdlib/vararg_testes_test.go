package stdlib

import (
	"os"
	"testing"
	luaapi "github.com/akzj/go-lua/internal/api"
)

func TestVarargTestes(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)
	data, err := os.ReadFile("../../../lua-master/testes/vararg.lua")
	if err != nil {
		t.Skipf("vararg.lua not found: %v", err)
	}
	if err := L.DoString(string(data)); err != nil {
		t.Fatalf("vararg.lua FAIL: %v", err)
	}
}
