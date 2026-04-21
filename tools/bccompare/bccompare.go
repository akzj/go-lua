// Package bccompare provides bytecode comparison tools for go-lua.
// It disassembles a Proto into the same text format as tools/disasm.lua
// so that go-lua output can be diff'd against C Lua reference output.
package bccompare

import (
	"fmt"
	"strings"

	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/opcode"
)

// DumpProto produces a text disassembly of a Proto in the same format
// as tools/disasm.lua (C Lua reference disassembler).
func DumpProto(p *object.Proto) string {
	var sb strings.Builder
	dumpProto(&sb, p, "")
	return sb.String()
}

func dumpProto(sb *strings.Builder, p *object.Proto, indent string) {
	src := "?"
	if p.Source != nil {
		src = p.Source.Data
	}

	fmt.Fprintf(sb, "%sfunction <%s:%d,%d> (%d instructions)\n",
		indent, src, p.LineDefined, p.LastLine, len(p.Code))
	fmt.Fprintf(sb, "%s%d params, %d slots, %d upvalues, %d locals, %d constants, %d functions\n",
		indent, p.NumParams, p.MaxStackSize, len(p.Upvalues),
		len(p.LocVars), len(p.Constants), len(p.Protos))

	// Instructions
	for pc, inst := range p.Code {
		op := opcode.GetOpCode(opcode.Instruction(inst))
		name := opcode.OpName(op)
		line := getLine(p, pc+1) // 1-based PC for line lookup
		mode := opcode.GetMode(op)
		args := formatArgs(opcode.Instruction(inst), mode)

		fmt.Fprintf(sb, "%s\t%d\t[%d]\t%-12s\t%s\n",
			indent, pc+1, line, name, args)
	}

	// Constants
	fmt.Fprintf(sb, "%sconstants (%d):\n", indent, len(p.Constants))
	for i, k := range p.Constants {
		fmt.Fprintf(sb, "%s\t%d\t%s\n", indent, i, fmtConstant(k))
	}

	// Locals
	fmt.Fprintf(sb, "%slocals (%d):\n", indent, len(p.LocVars))
	for i, lv := range p.LocVars {
		name := "?"
		if lv.Name != nil {
			name = lv.Name.Data
		}
		fmt.Fprintf(sb, "%s\t%d\t%s\t%d\t%d\n", indent, i, name, lv.StartPC, lv.EndPC)
	}

	// Upvalues
	fmt.Fprintf(sb, "%supvalues (%d):\n", indent, len(p.Upvalues))
	for i, uv := range p.Upvalues {
		name := "?"
		if uv.Name != nil {
			name = uv.Name.Data
		}
		instack := 0
		if uv.InStack {
			instack = 1
		}
		fmt.Fprintf(sb, "%s\t%d\t%s\t%d\t%d\t%d\n",
			indent, i, name, instack, uv.Idx, uv.Kind)
	}

	// Nested protos
	for _, sub := range p.Protos {
		fmt.Fprintln(sb)
		dumpProto(sb, sub, indent)
	}
}

func formatArgs(inst opcode.Instruction, mode opcode.OpMode) string {
	switch mode {
	case opcode.IABC:
		a := opcode.GetArgA(inst)
		b := opcode.GetArgB(inst)
		c := opcode.GetArgC(inst)
		k := opcode.GetArgK(inst)
		if k != 0 {
			return fmt.Sprintf("%d %d %d ; k=1", a, b, c)
		}
		return fmt.Sprintf("%d %d %d", a, b, c)
	case opcode.IVABC:
		a := opcode.GetArgA(inst)
		vb := opcode.GetArgVB(inst)
		vc := opcode.GetArgVC(inst)
		k := opcode.GetArgK(inst)
		if k != 0 {
			return fmt.Sprintf("%d %d %d ; k=1", a, vb, vc)
		}
		return fmt.Sprintf("%d %d %d", a, vb, vc)
	case opcode.IABx:
		return fmt.Sprintf("%d %d", opcode.GetArgA(inst), opcode.GetArgBx(inst))
	case opcode.IAsBx:
		return fmt.Sprintf("%d %d", opcode.GetArgA(inst), opcode.GetArgSBx(inst))
	case opcode.IAx:
		return fmt.Sprintf("%d", opcode.GetArgAx(inst))
	case opcode.ISJ:
		return fmt.Sprintf("%d", opcode.GetArgSJ(inst))
	default:
		return "?"
	}
}

func fmtConstant(v object.TValue) string {
	if v.IsNil() {
		return "nil"
	}
	if v.Tag() == object.TagFalse {
		return "false"
	}
	if v.Tag() == object.TagTrue {
		return "true"
	}
	if v.IsInteger() {
		return fmt.Sprintf("%d", v.Integer())
	}
	if v.IsFloat() {
		s := fmt.Sprintf("%.14g", v.Float())
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	}
	if v.IsString() {
		return fmt.Sprintf("%q", v.StringVal().Data)
	}
	return "?"
}

// getLine computes the absolute line number for 1-based PC.
func getLine(p *object.Proto, pc int) int {
	if len(p.LineInfo) == 0 || pc < 1 || pc > len(p.LineInfo) {
		return 0
	}
	line := p.LineDefined
	basepc := 0
	for _, ai := range p.AbsLineInfo {
		if ai.PC <= pc {
			line = ai.Line
			basepc = ai.PC
		}
	}
	for i := basepc + 1; i <= pc; i++ {
		if i <= len(p.LineInfo) {
			line += int(p.LineInfo[i-1]) // LineInfo is 0-based
		}
	}
	return line
}