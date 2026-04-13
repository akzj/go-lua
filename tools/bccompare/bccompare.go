// Package bccompare provides bytecode comparison tools for go-lua.
// It disassembles a Proto into the same text format as tools/disasm.lua
// so that go-lua output can be diff'd against C Lua reference output.
package bccompare

import (
	"fmt"
	"strings"

	objectapi "github.com/akzj/go-lua/internal/object/api"
	opcodeapi "github.com/akzj/go-lua/internal/opcode/api"
)

// DumpProto produces a text disassembly of a Proto in the same format
// as tools/disasm.lua (C Lua reference disassembler).
func DumpProto(p *objectapi.Proto) string {
	var sb strings.Builder
	dumpProto(&sb, p, "")
	return sb.String()
}

func dumpProto(sb *strings.Builder, p *objectapi.Proto, indent string) {
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
		op := opcodeapi.GetOpCode(opcodeapi.Instruction(inst))
		name := opcodeapi.OpName(op)
		line := getLine(p, pc+1) // 1-based PC for line lookup
		mode := opcodeapi.GetMode(op)
		args := formatArgs(opcodeapi.Instruction(inst), mode)

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

func formatArgs(inst opcodeapi.Instruction, mode opcodeapi.OpMode) string {
	switch mode {
	case opcodeapi.IABC:
		a := opcodeapi.GetArgA(inst)
		b := opcodeapi.GetArgB(inst)
		c := opcodeapi.GetArgC(inst)
		k := opcodeapi.GetArgK(inst)
		if k != 0 {
			return fmt.Sprintf("%d %d %d ; k=1", a, b, c)
		}
		return fmt.Sprintf("%d %d %d", a, b, c)
	case opcodeapi.IVABC:
		a := opcodeapi.GetArgA(inst)
		vb := opcodeapi.GetArgVB(inst)
		vc := opcodeapi.GetArgVC(inst)
		k := opcodeapi.GetArgK(inst)
		if k != 0 {
			return fmt.Sprintf("%d %d %d ; k=1", a, vb, vc)
		}
		return fmt.Sprintf("%d %d %d", a, vb, vc)
	case opcodeapi.IABx:
		return fmt.Sprintf("%d %d", opcodeapi.GetArgA(inst), opcodeapi.GetArgBx(inst))
	case opcodeapi.IAsBx:
		return fmt.Sprintf("%d %d", opcodeapi.GetArgA(inst), opcodeapi.GetArgSBx(inst))
	case opcodeapi.IAx:
		return fmt.Sprintf("%d", opcodeapi.GetArgAx(inst))
	case opcodeapi.ISJ:
		return fmt.Sprintf("%d", opcodeapi.GetArgSJ(inst))
	default:
		return "?"
	}
}

func fmtConstant(v objectapi.TValue) string {
	if v.IsNil() {
		return "nil"
	}
	if v.Tag() == objectapi.TagFalse {
		return "false"
	}
	if v.Tag() == objectapi.TagTrue {
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
func getLine(p *objectapi.Proto, pc int) int {
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