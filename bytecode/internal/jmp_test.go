package internal

import (
	"testing"
	"github.com/akzj/go-lua/vm/api"
)

func TestJMPEncoding(t *testing.T) {
	fs := &FuncState{Proto: &Prototype{}}
	
	// Forward jump of 5
	idx := fs.emitSJ(5)
	inst := api.Instruction(fs.Proto.code[idx])
	sbx := api.GetsBx(inst)
	t.Logf("emitSJ(5): idx=%d, code=0x%x, decoded sbx=%d", idx, fs.Proto.code[idx], sbx)
	
	// Test backward jump
	fs2 := &FuncState{Proto: &Prototype{}}
	fs2.emitABC(1, 0, 0, 0) // some instruction
	fs2.emitABC(1, 0, 0, 0) // some instruction
	fs2.emitABC(1, 0, 0, 0) // some instruction
	// pc is now 3, after 3 instructions
	// Emit backward jump with target = pc (3) - backIdx (3) - 1 = -1 (jump to previous instruction)
	backIdx := fs2.emitSJ(-1)
	inst2 := api.Instruction(fs2.Proto.code[backIdx])
	sbx2 := api.GetsBx(inst2)
	t.Logf("emitSJ(-1): idx=%d, pc=%d, code=0x%x, decoded sbx=%d", backIdx, fs2.pc, fs2.Proto.code[backIdx], sbx2)
}
