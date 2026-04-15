package api

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func TestFloatFor(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)
	defer L.Close()
	err := L.DoFile("/tmp/test_narrow.lua")
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
}
