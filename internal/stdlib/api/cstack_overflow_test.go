package api

import (
	"testing"
	"time"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func TestCStackOverflow(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)
	code := `local function loop() assert(pcall(loop)) end
             local err, msg = xpcall(loop, loop)
             assert(not err and string.find(msg, "error"))`
	done := make(chan error, 1)
	go func() { done <- L.DoString(code) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TIMEOUT")
	}
}