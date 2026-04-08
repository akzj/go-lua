package internal

import (
    "testing"
    "github.com/akzj/go-lua/parse"
    "github.com/akzj/go-lua/opcodes"
    bcapi "github.com/akzj/go-lua/bytecode/api"
)

func TestTableConstructorBytecode(t *testing.T) {
    code := `local t = {1, 2, 3}`
    parser := parse.NewParser()
    chunk, err := parser.Parse(code)
    if err != nil {
        t.Fatal("parse:", err)
    }
    c := NewCompiler("test")
    proto, err := c.Compile(chunk)
    if err != nil {
        t.Fatal("compile:", err)
    }
    t.Logf("=== {1,2,3} bytecode ===")
    t.Logf("maxstacksize=%d nconstants=%d", proto.MaxStackSize(), len(proto.GetConstants()))
    for i, instr := range proto.GetCode() {
        op := opcodes.OpCode(instr & 0x7F)
        name := opcodes.Name(op)
        A := int(instr>>6) & 0xFF
        B := int(instr>>23) & 0x1FF
        C := int(instr>>14) & 0x1FF
        Bx := int(instr>>14) & 0x3FFFF
        extra := ""
        if name == "SETI" {
            extra = " [SETI R[A=" + itoa(A) + "] idx=B=" + itoa(B) + " srcReg=C=" + itoa(C) + "]"
        }
        if name == "LOADK" && Bx < len(proto.GetConstants()) {
            kc := proto.GetConstants()[Bx]
            if kc.Type == bcapi.ConstInteger {
                extra = " -> const[" + itoa(Bx) + "]=" + itoa(int(kc.Int))
            }
        }
        t.Logf("[%d] %s A=%d B=%d C=%d Bx=%d%s", i, name, A, B, C, Bx, extra)
    }
    t.Logf("Constants:")
    for i, kc := range proto.GetConstants() {
        if kc.Type == bcapi.ConstInteger {
            t.Logf("  [%d] int=%d", i, kc.Int)
        }
    }
}

func itoa(i int) string {
    if i < 0 { return "-" + uitoa(-i) }
    return uitoa(i)
}
func uitoa(i int) string {
    if i == 0 { return "0" }
    s := ""
    for i > 0 { s = string('0'+byte(i%10)) + s; i /= 10 }
    return s
}
