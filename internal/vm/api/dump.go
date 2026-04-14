package api

import (
	"encoding/binary"
	"math"

	objectapi "github.com/akzj/go-lua/internal/object/api"
)

// DumpProto serializes a Proto to Lua 5.5 binary chunk format.
// If strip is true, debug information is omitted.
// Mirrors: luaU_dump in ldump.c
func DumpProto(p *objectapi.Proto, strip bool) []byte {
	d := &dumpState{
		strip: strip,
		strs:  make(map[string]uint64),
	}
	d.dumpHeader()
	d.dumpByte(byte(len(p.Upvalues)))
	d.dumpFunction(p)
	return d.buf
}

type dumpState struct {
	buf   []byte
	strip bool
	strs  map[string]uint64 // string dedup: content → 1-based index
	nstr  uint64            // count of saved strings
}

func (d *dumpState) dumpBlock(data []byte) {
	d.buf = append(d.buf, data...)
}

func (d *dumpState) dumpByte(b byte) {
	d.buf = append(d.buf, b)
}

// dumpAlign pads to align-byte boundary based on absolute offset.
func (d *dumpState) dumpAlign(align int) {
	offset := len(d.buf)
	padding := align - (offset % align)
	if padding < align {
		for i := 0; i < padding; i++ {
			d.buf = append(d.buf, 0)
		}
	}
}

// dumpVarint encodes an unsigned integer using MSB Varint encoding.
// High bit = continuation flag, most-significant byte first.
func (d *dumpState) dumpVarint(x uint64) {
	var buf [10]byte // max 10 bytes for 64-bit
	n := 1
	buf[9] = byte(x & 0x7f) // least-significant byte (no continuation)
	for x >>= 7; x != 0; x >>= 7 {
		n++
		buf[10-n] = byte((x & 0x7f) | 0x80) // continuation bit set
	}
	d.dumpBlock(buf[10-n:])
}

func (d *dumpState) dumpInt(x int) {
	d.dumpVarint(uint64(x))
}

// dumpInteger encodes a signed integer using zigzag + varint.
// Non-negative x → 2*x; negative x → 2*(-x) - 1
func (d *dumpState) dumpInteger(x int64) {
	var cx uint64
	if x >= 0 {
		cx = 2 * uint64(x)
	} else {
		cx = 2*uint64(^x) + 1
	}
	d.dumpVarint(cx)
}

// dumpNumber writes a float64 in native (little-endian) format.
func (d *dumpState) dumpNumber(x float64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(x))
	d.dumpBlock(buf[:])
}

// dumpString encodes a string with dedup support.
// NULL → size=0, index=0
// Previously saved → size=0, index=saved_index
// New string → size=len+1, content including trailing \0
func (d *dumpState) dumpString(s *objectapi.LuaString) {
	if s == nil {
		d.dumpVarint(0) // size = 0
		d.dumpVarint(0) // index = 0 (NULL)
		return
	}
	data := s.String()
	if idx, ok := d.strs[data]; ok {
		d.dumpVarint(0)   // size = 0 (reuse)
		d.dumpVarint(idx) // index of saved string
		return
	}
	// New string: write size (len+1) then content + trailing \0
	d.dumpVarint(uint64(len(data)) + 1)
	d.dumpBlock([]byte(data))
	d.dumpByte(0) // trailing \0
	// Save for dedup
	d.nstr++
	d.strs[data] = d.nstr
}

func (d *dumpState) dumpCode(p *objectapi.Proto) {
	d.dumpInt(len(p.Code))
	d.dumpAlign(4) // align to sizeof(Instruction)
	for _, inst := range p.Code {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], inst)
		d.dumpBlock(buf[:])
	}
}

func (d *dumpState) dumpConstants(p *objectapi.Proto) {
	d.dumpInt(len(p.Constants))
	for _, k := range p.Constants {
		tt := byte(k.Tt)
		d.dumpByte(tt)
		switch k.Tt {
		case objectapi.TagFloat:
			d.dumpNumber(k.Float())
		case objectapi.TagInteger:
			d.dumpInteger(k.Integer())
		case objectapi.TagShortStr, objectapi.TagLongStr:
			d.dumpString(k.StringVal())
		// TagNil, TagFalse, TagTrue: no payload
		}
	}
}

func (d *dumpState) dumpUpvalues(p *objectapi.Proto) {
	d.dumpInt(len(p.Upvalues))
	for _, uv := range p.Upvalues {
		if uv.InStack {
			d.dumpByte(1)
		} else {
			d.dumpByte(0)
		}
		d.dumpByte(uv.Idx)
		d.dumpByte(uv.Kind)
	}
}

func (d *dumpState) dumpProtos(p *objectapi.Proto) {
	d.dumpInt(len(p.Protos))
	for _, child := range p.Protos {
		d.dumpFunction(child)
	}
}

func (d *dumpState) dumpDebug(p *objectapi.Proto) {
	// lineinfo
	n := 0
	if !d.strip {
		n = len(p.LineInfo)
	}
	d.dumpInt(n)
	if n > 0 {
		for _, li := range p.LineInfo[:n] {
			d.dumpByte(byte(li))
		}
	}

	// abslineinfo
	n = 0
	if !d.strip {
		n = len(p.AbsLineInfo)
	}
	d.dumpInt(n)
	if n > 0 {
		d.dumpAlign(4) // align to sizeof(int)
		for _, ali := range p.AbsLineInfo[:n] {
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], uint32(int32(ali.PC)))
			d.dumpBlock(buf[:])
			binary.LittleEndian.PutUint32(buf[:], uint32(int32(ali.Line)))
			d.dumpBlock(buf[:])
		}
	}

	// locvars
	n = 0
	if !d.strip {
		n = len(p.LocVars)
	}
	d.dumpInt(n)
	for i := 0; i < n; i++ {
		d.dumpString(p.LocVars[i].Name)
		d.dumpInt(p.LocVars[i].StartPC)
		d.dumpInt(p.LocVars[i].EndPC)
	}

	// upvalue names
	n = 0
	if !d.strip {
		n = len(p.Upvalues)
	}
	d.dumpInt(n)
	for i := 0; i < n; i++ {
		d.dumpString(p.Upvalues[i].Name)
	}
}

func (d *dumpState) dumpFunction(p *objectapi.Proto) {
	d.dumpInt(p.LineDefined)
	d.dumpInt(p.LastLine)
	d.dumpByte(p.NumParams)
	d.dumpByte(p.Flag)
	d.dumpByte(p.MaxStackSize)
	d.dumpCode(p)
	d.dumpConstants(p)
	d.dumpUpvalues(p)
	d.dumpProtos(p)
	if d.strip {
		d.dumpString(nil)
	} else {
		d.dumpString(p.Source)
	}
	d.dumpDebug(p)
}

// dumpNumInfo writes: 1 byte sizeof(T) + value in little-endian
func (d *dumpState) dumpNumInfo4(size byte, val uint32) {
	d.dumpByte(size)
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], val)
	d.dumpBlock(buf[:size])
}

func (d *dumpState) dumpNumInfo8(size byte, val uint64) {
	d.dumpByte(size)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], val)
	d.dumpBlock(buf[:size])
}

func (d *dumpState) dumpHeader() {
	// LUA_SIGNATURE
	d.dumpBlock([]byte("\x1bLua"))
	// LUAC_VERSION (5.5 = 0x55)
	d.dumpByte(0x55)
	// LUAC_FORMAT
	d.dumpByte(0x00)
	// LUAC_DATA
	d.dumpBlock([]byte("\x19\x93\r\n\x1a\n"))

	// dumpNumInfo(int, LUAC_INT=-0x5678)
	d.dumpByte(4) // sizeof(int)
	var buf4 [4]byte
	luacInt32 := int32(-0x5678)
	binary.LittleEndian.PutUint32(buf4[:], uint32(luacInt32))
	d.dumpBlock(buf4[:])

	// dumpNumInfo(Instruction, LUAC_INST=0x12345678)
	d.dumpByte(4) // sizeof(Instruction)
	binary.LittleEndian.PutUint32(buf4[:], 0x12345678)
	d.dumpBlock(buf4[:])

	// dumpNumInfo(lua_Integer, LUAC_INT=-0x5678)
	d.dumpByte(8) // sizeof(lua_Integer)
	var buf8 [8]byte
	luacInt64 := int64(-0x5678)
	binary.LittleEndian.PutUint64(buf8[:], uint64(luacInt64))
	d.dumpBlock(buf8[:])

	// dumpNumInfo(lua_Number, LUAC_NUM=-370.5)
	d.dumpByte(8) // sizeof(lua_Number)
	binary.LittleEndian.PutUint64(buf8[:], math.Float64bits(-370.5))
	d.dumpBlock(buf8[:])
}