package lzio

import (
	"testing"

	"github.com/akzj/go-lua/internal/lstate"
)

func TestInitBuffer(t *testing.T) {
	var buff Mbuffer
	InitBuffer(&buff)
	if buff.Buffer != nil {
		t.Error("InitBuffer: Buffer should be nil")
	}
	if buff.BuffSize != 0 {
		t.Error("InitBuffer: BuffSize should be 0")
	}
	if buff.N != 0 {
		t.Error("InitBuffer: N should be 0")
	}
}

func TestResizeBuffer(t *testing.T) {
	L := &lstate.LuaState{}
	var buff Mbuffer
	InitBuffer(&buff)

	ResizeBuffer(L, &buff, 100)
	if buff.BuffSize != 100 {
		t.Errorf("ResizeBuffer: expected size 100, got %d", buff.BuffSize)
	}
	if len(buff.Buffer) != 100 {
		t.Errorf("ResizeBuffer: expected buffer len 100, got %d", len(buff.Buffer))
	}
}

func TestResetBuffer(t *testing.T) {
	L := &lstate.LuaState{}
	var buff Mbuffer
	InitBuffer(&buff)
	ResizeBuffer(L, &buff, 100)
	buff.N = 50

	ResetBuffer(&buff)
	if buff.N != 0 {
		t.Errorf("ResetBuffer: expected N 0, got %d", buff.N)
	}
}

func TestBuffRemove(t *testing.T) {
	var buff Mbuffer
	buff.N = 10
	BuffRemove(&buff, 3)
	if buff.N != 7 {
		t.Errorf("BuffRemove: expected N 7, got %d", buff.N)
	}
}

func TestInit(t *testing.T) {
	L := &lstate.LuaState{}
	var z ZIO

	reader := func(L *lstate.LuaState, data interface{}, size *int64) []byte {
		return nil
	}

	Init(L, &z, reader, nil)
	if z.L != L {
		t.Error("Init: L not set correctly")
	}
	if z.Reader == nil {
		t.Error("Init: Reader should not be nil")
	}
	if z.N != 0 {
		t.Error("Init: N should be 0")
	}
}

func TestEOZ(t *testing.T) {
	if EOZ != -1 {
		t.Errorf("EOZ should be -1, got %d", EOZ)
	}
}

func TestMinBuffer(t *testing.T) {
	if MinBuffer != 32 {
		t.Errorf("MinBuffer should be 32, got %d", MinBuffer)
	}
}
