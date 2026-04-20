package api

import (
	"encoding/binary"
	"fmt"
	"math"

	closureapi "github.com/akzj/go-lua/internal/closure/api"
	luastringapi "github.com/akzj/go-lua/internal/luastring/api"
	objectapi "github.com/akzj/go-lua/internal/object/api"
	stateapi "github.com/akzj/go-lua/internal/state/api"
)

// UndumpProto deserializes a Lua 5.5 binary chunk into a Proto + LClosure.
// Returns the LClosure ready to execute.
// Mirrors: luaU_undump in lundump.c
func UndumpProto(L *stateapi.LuaState, data []byte, name string) (*closureapi.LClosure, error) {
	s := &loadState{
		data:   data,
		offset: 0,
		name:   name,
		strs:   make([]*objectapi.LuaString, 0, 16),
		L:      L,
	}

	if err := s.checkHeader(); err != nil {
		return nil, fmt.Errorf("%s: %s", name, err.Error())
	}

	nupvals := int(s.loadByte())
	if s.err != nil {
		return nil, fmt.Errorf("%s: %s", name, s.err.Error())
	}

	p := &objectapi.Proto{}
	s.loadFunction(p)
	if s.err != nil {
		return nil, fmt.Errorf("%s: %s", name, s.err.Error())
	}
	L.Global.LinkGC(p) // V5: register proto in allgc chain

	if nupvals != len(p.Upvalues) {
		return nil, fmt.Errorf("%s: bad binary format (corrupted chunk)", name)
	}

	cl := closureapi.NewLClosure(p, nupvals)
	L.Global.LinkGC(cl) // V5: register closure in allgc chain
	closureapi.InitUpvals(cl)
	// Link newly created upvalues to allgc for proper GC tracking
	for _, uv := range cl.UpVals {
		if uv != nil {
			L.Global.LinkGC(uv)
		}
	}
	return cl, nil
}

type loadState struct {
	data   []byte
	offset int
	name   string
	err    error
	strs   []*objectapi.LuaString // 1-based string reuse list
	L      *stateapi.LuaState
}

func (s *loadState) error(msg string) {
	if s.err == nil {
		s.err = fmt.Errorf("bad binary format (%s)", msg)
	}
}

func (s *loadState) loadBlock(n int) []byte {
	if s.err != nil {
		return nil
	}
	if s.offset+n > len(s.data) {
		s.error("truncated chunk")
		return nil
	}
	b := s.data[s.offset : s.offset+n]
	s.offset += n
	return b
}

func (s *loadState) loadByte() byte {
	b := s.loadBlock(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (s *loadState) loadAlign(align int) {
	padding := align - (s.offset % align)
	if padding < align {
		s.loadBlock(padding)
	}
}

// loadVarint decodes an MSB Varint unsigned integer.
func (s *loadState) loadVarint() uint64 {
	if s.err != nil {
		return 0
	}
	var x uint64
	for {
		b := s.loadByte()
		if s.err != nil {
			return 0
		}
		x = (x << 7) | uint64(b&0x7f)
		if (b & 0x80) == 0 {
			break
		}
	}
	return x
}

func (s *loadState) loadInt() int {
	return int(s.loadVarint())
}

func (s *loadState) loadSize() int {
	return int(s.loadVarint())
}

// loadInteger decodes a zigzag-encoded signed integer.
func (s *loadState) loadInteger() int64 {
	cx := s.loadVarint()
	if (cx & 1) != 0 {
		return int64(^(cx >> 1))
	}
	return int64(cx >> 1)
}

func (s *loadState) loadNumber() float64 {
	b := s.loadBlock(8)
	if b == nil {
		return 0
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(b))
}

func (s *loadState) loadString() *objectapi.LuaString {
	if s.err != nil {
		return nil
	}
	size := s.loadSize()
	if size == 0 {
		// Reuse a previously saved string
		idx := s.loadVarint()
		if idx == 0 {
			return nil // NULL string
		}
		i := int(idx) - 1 // 1-based → 0-based
		if i < 0 || i >= len(s.strs) {
			s.error("invalid string index")
			return nil
		}
		return s.strs[i]
	}
	// New string: size includes trailing \0, real length = size-1
	realLen := size - 1
	b := s.loadBlock(realLen + 1) // read content + \0
	if b == nil {
		return nil
	}
	str := string(b[:realLen]) // exclude trailing \0

	// Intern the string
	st := s.L.Global.StringTable.(*luastringapi.StringTable)
	ls := st.Intern(str)

	// Save for reuse
	s.strs = append(s.strs, ls)
	return ls
}

func (s *loadState) loadCode(p *objectapi.Proto) {
	n := s.loadInt()
	s.loadAlign(4) // align to sizeof(Instruction)
	p.Code = make([]uint32, n)
	for i := 0; i < n; i++ {
		b := s.loadBlock(4)
		if b == nil {
			return
		}
		p.Code[i] = binary.LittleEndian.Uint32(b)
	}
}

func (s *loadState) loadConstants(p *objectapi.Proto) {
	n := s.loadInt()
	p.Constants = make([]objectapi.TValue, n)
	for i := 0; i < n; i++ {
		t := s.loadByte()
		if s.err != nil {
			return
		}
		switch objectapi.Tag(t) {
		case objectapi.TagNil:
			p.Constants[i] = objectapi.Nil
		case objectapi.TagFalse:
			p.Constants[i] = objectapi.False
		case objectapi.TagTrue:
			p.Constants[i] = objectapi.True
		case objectapi.TagFloat:
			p.Constants[i] = objectapi.MakeFloat(s.loadNumber())
		case objectapi.TagInteger:
			p.Constants[i] = objectapi.MakeInteger(s.loadInteger())
		case objectapi.TagShortStr, objectapi.TagLongStr:
			ls := s.loadString()
			if ls == nil {
				s.error("bad format for constant string")
				return
			}
			p.Constants[i] = objectapi.MakeString(ls)
		default:
			s.error("invalid constant")
			return
		}
	}
}

func (s *loadState) loadUpvalues(p *objectapi.Proto) {
	n := s.loadInt()
	p.Upvalues = make([]objectapi.UpvalDesc, n)
	for i := 0; i < n; i++ {
		p.Upvalues[i].InStack = s.loadByte() != 0
		p.Upvalues[i].Idx = s.loadByte()
		p.Upvalues[i].Kind = s.loadByte()
	}
}

func (s *loadState) loadProtos(p *objectapi.Proto) {
	n := s.loadInt()
	p.Protos = make([]*objectapi.Proto, n)
	for i := 0; i < n; i++ {
		p.Protos[i] = &objectapi.Proto{}
		s.loadFunction(p.Protos[i])
		s.L.Global.LinkGC(p.Protos[i]) // V5: register nested proto in allgc chain
	}
}

func (s *loadState) loadDebug(p *objectapi.Proto) {
	// lineinfo
	n := s.loadInt()
	if n > 0 {
		p.LineInfo = make([]int8, n)
		b := s.loadBlock(n)
		if b == nil {
			return
		}
		for i := 0; i < n; i++ {
			p.LineInfo[i] = int8(b[i])
		}
	}

	// abslineinfo
	n = s.loadInt()
	if n > 0 {
		s.loadAlign(4) // align to sizeof(int)
		p.AbsLineInfo = make([]objectapi.AbsLineInfo, n)
		for i := 0; i < n; i++ {
			b := s.loadBlock(8) // 2 x int32
			if b == nil {
				return
			}
			p.AbsLineInfo[i].PC = int(int32(binary.LittleEndian.Uint32(b[:4])))
			p.AbsLineInfo[i].Line = int(int32(binary.LittleEndian.Uint32(b[4:])))
		}
	}

	// locvars
	n = s.loadInt()
	if n > 0 {
		p.LocVars = make([]objectapi.LocVar, n)
		for i := 0; i < n; i++ {
			p.LocVars[i].Name = s.loadString()
			p.LocVars[i].StartPC = s.loadInt()
			p.LocVars[i].EndPC = s.loadInt()
		}
	}

	// upvalue names
	n = s.loadInt()
	if n > 0 {
		for i := 0; i < n && i < len(p.Upvalues); i++ {
			p.Upvalues[i].Name = s.loadString()
		}
	}
}

func (s *loadState) loadFunction(p *objectapi.Proto) {
	p.LineDefined = s.loadInt()
	p.LastLine = s.loadInt()
	p.NumParams = s.loadByte()
	p.Flag = s.loadByte() &^ 0x04 // clear PF_FIXED flag
	p.MaxStackSize = s.loadByte()
	s.loadCode(p)
	s.loadConstants(p)
	s.loadUpvalues(p)
	s.loadProtos(p)
	p.Source = s.loadString()
	s.loadDebug(p)
}

func (s *loadState) checkHeader() error {
	// Check signature (first byte already consumed by caller in C Lua,
	// but we have the full data here)
	sig := s.loadBlock(4)
	if sig == nil {
		return s.err
	}
	if string(sig) != "\x1bLua" {
		s.error("not a binary chunk")
		return s.err
	}

	// Version
	if s.loadByte() != 0x55 {
		s.error("version mismatch")
		return s.err
	}

	// Format
	if s.loadByte() != 0x00 {
		s.error("format mismatch")
		return s.err
	}

	// LUAC_DATA
	data := s.loadBlock(6)
	if data == nil {
		return s.err
	}
	if string(data) != "\x19\x93\r\n\x1a\n" {
		s.error("corrupted chunk")
		return s.err
	}

	// checknum(int, LUAC_INT)
	if s.loadByte() != 4 { // sizeof(int)
		s.error("int size mismatch")
		return s.err
	}
	intBytes := s.loadBlock(4)
	if intBytes == nil {
		return s.err
	}
	luacInt := int32(-0x5678)
	if int32(binary.LittleEndian.Uint32(intBytes)) != luacInt {
		s.error("int format mismatch")
		return s.err
	}

	// checknum(Instruction, LUAC_INST)
	if s.loadByte() != 4 { // sizeof(Instruction)
		s.error("instruction size mismatch")
		return s.err
	}
	instBytes := s.loadBlock(4)
	if instBytes == nil {
		return s.err
	}
	if binary.LittleEndian.Uint32(instBytes) != 0x12345678 {
		s.error("instruction format mismatch")
		return s.err
	}

	// checknum(lua_Integer, LUAC_INT)
	if s.loadByte() != 8 { // sizeof(lua_Integer)
		s.error("Lua integer size mismatch")
		return s.err
	}
	integerBytes := s.loadBlock(8)
	if integerBytes == nil {
		return s.err
	}
	luacInt64 := int64(-0x5678)
	if int64(binary.LittleEndian.Uint64(integerBytes)) != luacInt64 {
		s.error("Lua integer format mismatch")
		return s.err
	}

	// checknum(lua_Number, LUAC_NUM)
	if s.loadByte() != 8 { // sizeof(lua_Number)
		s.error("Lua number size mismatch")
		return s.err
	}
	numBytes := s.loadBlock(8)
	if numBytes == nil {
		return s.err
	}
	if math.Float64frombits(binary.LittleEndian.Uint64(numBytes)) != -370.5 {
		s.error("Lua number format mismatch")
		return s.err
	}

	return s.err
}