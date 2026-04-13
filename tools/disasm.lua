-- disasm.lua — Lua 5.5.1 bytecode disassembler
-- Parses string.dump output and prints instructions in a diff-friendly format.
-- Usage: lua disasm.lua file.lua
--    or: cat code.lua | lua disasm.lua -

local opnames = {
  [0]="MOVE","LOADI","LOADF","LOADK","LOADKX","LOADFALSE","LFALSESKIP",
  "LOADTRUE","LOADNIL","GETUPVAL","SETUPVAL","GETTABUP","GETTABLE",
  "GETI","GETFIELD","SETTABUP","SETTABLE","SETI","SETFIELD","NEWTABLE",
  "SELF","ADDI","ADDK","SUBK","MULK","MODK","POWK","DIVK","IDIVK",
  "BANDK","BORK","BXORK","SHLI","SHRI","ADD","SUB","MUL","MOD","POW",
  "DIV","IDIV","BAND","BOR","BXOR","SHL","SHR","MMBIN","MMBINI","MMBINK",
  "UNM","BNOT","NOT","LEN","CONCAT","CLOSE","TBC","JMP","EQ","LT","LE",
  "EQK","EQI","LTI","LEI","GTI","GEI","TEST","TESTSET","CALL","TAILCALL",
  "RETURN","RETURN0","RETURN1","FORLOOP","FORPREP","TFORPREP","TFORCALL",
  "TFORLOOP","SETLIST","CLOSURE","VARARG","GETVARG","ERRNNIL","VARARGPREP",
  "EXTRAARG",
}

-- Instruction format modes extracted from lopcodes.c
-- 0=iABC, 1=ivABC, 2=iABx, 3=iAsBx, 4=iAx, 5=isJ
local opmodes = {
  [0] =0, -- MOVE      iABC
  3,       -- LOADI     iAsBx
  3,       -- LOADF     iAsBx
  2,       -- LOADK     iABx
  2,       -- LOADKX    iABx
  0,       -- LOADFALSE iABC
  0,       -- LFALSESKIP iABC
  0,       -- LOADTRUE  iABC
  0,       -- LOADNIL   iABC
  0,       -- GETUPVAL  iABC
  0,       -- SETUPVAL  iABC
  0,       -- GETTABUP  iABC
  0,       -- GETTABLE  iABC
  0,       -- GETI      iABC
  0,       -- GETFIELD  iABC
  0,       -- SETTABUP  iABC
  0,       -- SETTABLE  iABC
  0,       -- SETI      iABC
  0,       -- SETFIELD  iABC
  1,       -- NEWTABLE  ivABC
  0,       -- SELF      iABC
  0,       -- ADDI      iABC
  0,       -- ADDK      iABC
  0,       -- SUBK      iABC
  0,       -- MULK      iABC
  0,       -- MODK      iABC
  0,       -- POWK      iABC
  0,       -- DIVK      iABC
  0,       -- IDIVK     iABC
  0,       -- BANDK     iABC
  0,       -- BORK      iABC
  0,       -- BXORK     iABC
  0,       -- SHLI      iABC
  0,       -- SHRI      iABC
  0,       -- ADD       iABC
  0,       -- SUB       iABC
  0,       -- MUL       iABC
  0,       -- MOD       iABC
  0,       -- POW       iABC
  0,       -- DIV       iABC
  0,       -- IDIV      iABC
  0,       -- BAND      iABC
  0,       -- BOR       iABC
  0,       -- BXOR      iABC
  0,       -- SHL       iABC
  0,       -- SHR       iABC
  0,       -- MMBIN     iABC
  0,       -- MMBINI    iABC
  0,       -- MMBINK    iABC
  0,       -- UNM       iABC
  0,       -- BNOT      iABC
  0,       -- NOT       iABC
  0,       -- LEN       iABC
  0,       -- CONCAT    iABC
  0,       -- CLOSE     iABC
  0,       -- TBC       iABC
  5,       -- JMP       isJ
  0,       -- EQ        iABC
  0,       -- LT        iABC
  0,       -- LE        iABC
  0,       -- EQK       iABC
  0,       -- EQI       iABC
  0,       -- LTI       iABC
  0,       -- LEI       iABC
  0,       -- GTI       iABC
  0,       -- GEI       iABC
  0,       -- TEST      iABC
  0,       -- TESTSET   iABC
  0,       -- CALL      iABC
  0,       -- TAILCALL  iABC
  0,       -- RETURN    iABC
  0,       -- RETURN0   iABC
  0,       -- RETURN1   iABC
  2,       -- FORLOOP   iABx
  2,       -- FORPREP   iABx
  2,       -- TFORPREP  iABx
  0,       -- TFORCALL  iABC
  2,       -- TFORLOOP  iABx
  1,       -- SETLIST   ivABC
  2,       -- CLOSURE   iABx
  0,       -- VARARG    iABC
  0,       -- GETVARG   iABC
  2,       -- ERRNNIL   iABx
  0,       -- VARARGPREP iABC
  4,       -- EXTRAARG  iAx
}

-- Bit field sizes and positions (from lopcodes.h)
local SIZE_OP, SIZE_A, SIZE_B, SIZE_C = 7, 8, 8, 8
local SIZE_Bx = SIZE_C + SIZE_B + 1  -- 17
local SIZE_Ax = SIZE_Bx + SIZE_A     -- 25
local SIZE_VB, SIZE_VC = 6, 10

local POS_OP = 0
local POS_A  = POS_OP + SIZE_OP  -- 7
local POS_K  = POS_A + SIZE_A    -- 15
local POS_B  = POS_K + 1         -- 16
local POS_C  = POS_B + SIZE_B    -- 24
local POS_Bx = POS_K             -- 15
local POS_Ax = POS_A             -- 7
local POS_VB = POS_K + 1         -- 16
local POS_VC = POS_VB + SIZE_VB  -- 22

local MaxArgBx = (1 << SIZE_Bx) - 1
local MaxArgSJ = (1 << SIZE_Ax) - 1
local MaxArgC  = (1 << SIZE_C) - 1
local OFFSET_sBx = MaxArgBx >> 1   -- 65535
local OFFSET_sJ  = MaxArgSJ >> 1   -- 16777215
local OFFSET_sC  = MaxArgC >> 1    -- 127

local function mask(n) return (1 << n) - 1 end
local function getfield(i, pos, size) return (i >> pos) & mask(size) end

local function getOP(i) return getfield(i, POS_OP, SIZE_OP) end
local function getA(i)  return getfield(i, POS_A, SIZE_A) end
local function getB(i)  return getfield(i, POS_B, SIZE_B) end
local function getC(i)  return getfield(i, POS_C, SIZE_C) end
local function getK(i)  return getfield(i, POS_K, 1) end
local function getBx(i) return getfield(i, POS_Bx, SIZE_Bx) end
local function getsBx(i) return getBx(i) - OFFSET_sBx end
local function getAx(i)  return getfield(i, POS_Ax, SIZE_Ax) end
local function getsJ(i)  return getfield(i, POS_Ax, SIZE_Ax) - OFFSET_sJ end
local function getsC(i)  return getC(i) - OFFSET_sC end
local function getsB(i)  return getB(i) - OFFSET_sC end
local function getVB(i)  return getfield(i, POS_VB, SIZE_VB) end
local function getVC(i)  return getfield(i, POS_VC, SIZE_VC) end

-- ---------------------------------------------------------------------------
-- Binary reader for string.dump output
-- ---------------------------------------------------------------------------
local Reader = {}
Reader.__index = Reader

function Reader.new(data)
  return setmetatable({data=data, pos=1}, Reader)
end

function Reader:byte()
  local b = string.byte(self.data, self.pos)
  self.pos = self.pos + 1
  return b
end

function Reader:int32()
  local a,b,c,d = string.byte(self.data, self.pos, self.pos+3)
  self.pos = self.pos + 4
  return a | (b << 8) | (c << 16) | (d << 24)
end

function Reader:double()
  local s = string.sub(self.data, self.pos, self.pos+7)
  self.pos = self.pos + 8
  return string.unpack("<d", s)
end

function Reader:varint()
  local x = 0
  local b
  repeat
    b = self:byte()
    x = (x << 7) | (b & 0x7f)
  until (b & 0x80) == 0
  return x
end

function Reader:align(n)
  local off = (self.pos - 1) % n
  if off ~= 0 then
    self.pos = self.pos + (n - off)
  end
end

-- String table for reuse tracking
local stringTable = {}

function Reader:string()
  local size = self:varint()
  if size == 0 then
    local idx = self:varint()
    if idx == 0 then return nil end
    return stringTable[idx]
  end
  size = size - 1  -- real size (without trailing \0)
  local s = string.sub(self.data, self.pos, self.pos + size - 1)
  self.pos = self.pos + size + 1  -- skip string + trailing \0
  stringTable[#stringTable + 1] = s
  return s
end

function Reader:integer()
  local cx = self:varint()
  if cx % 2 == 0 then
    return cx // 2
  else
    return -(((cx - 1) // 2) + 1)
  end
end

-- ---------------------------------------------------------------------------
-- Parse a function proto from the binary dump
-- ---------------------------------------------------------------------------
local function readProto(r, parentSource)
  local proto = {}
  proto.lineDefined = r:varint()
  proto.lastLine = r:varint()
  proto.numParams = r:byte()
  proto.flag = r:byte()
  proto.maxStack = r:byte()

  -- Code
  local ncode = r:varint()
  r:align(4)
  proto.code = {}
  for i = 1, ncode do
    proto.code[i] = r:int32()
  end

  -- Constants
  local nk = r:varint()
  proto.constants = {}
  for i = 1, nk do
    local tt = r:byte()
    if tt == 0x00 then
      proto.constants[i] = {type="nil"}
    elseif tt == 0x01 then
      proto.constants[i] = {type="false"}
    elseif tt == 0x11 then
      proto.constants[i] = {type="true"}
    elseif tt == 0x03 then
      proto.constants[i] = {type="int", value=r:integer()}
    elseif tt == 0x13 then
      proto.constants[i] = {type="float", value=r:double()}
    elseif tt == 0x04 or tt == 0x14 then
      proto.constants[i] = {type="string", value=r:string()}
    else
      proto.constants[i] = {type="unknown:" .. tt}
    end
  end

  -- Upvalues
  local nups = r:varint()
  proto.upvalues = {}
  for i = 1, nups do
    proto.upvalues[i] = {
      instack = r:byte(),
      idx = r:byte(),
      kind = r:byte(),
    }
  end

  -- Nested protos
  local np = r:varint()
  proto.protos = {}
  for i = 1, np do
    proto.protos[i] = readProto(r, proto.source or parentSource)
  end

  -- Source
  proto.source = r:string() or parentSource

  -- Debug: line info (signed bytes)
  local nline = r:varint()
  proto.lineinfo = {}
  for i = 1, nline do
    local b = r:byte()
    if b >= 128 then b = b - 256 end
    proto.lineinfo[i] = b
  end

  -- Abs line info
  local nabsline = r:varint()
  proto.abslineinfo = {}
  if nabsline > 0 then
    r:align(4)
    for i = 1, nabsline do
      proto.abslineinfo[i] = {pc=r:int32(), line=r:int32()}
    end
  end

  -- Local vars
  local nlocvars = r:varint()
  proto.locvars = {}
  for i = 1, nlocvars do
    proto.locvars[i] = {
      name = r:string(),
      startpc = r:varint(),
      endpc = r:varint(),
    }
  end

  -- Upvalue names
  local nupnames = r:varint()
  for i = 1, nupnames do
    if proto.upvalues[i] then
      proto.upvalues[i].name = r:string()
    else
      r:string()  -- skip
    end
  end

  return proto
end

-- ---------------------------------------------------------------------------
-- Compute absolute line number from delta + abslineinfo
-- ---------------------------------------------------------------------------
local function getLine(proto, pc)
  if #proto.lineinfo == 0 then return 0 end
  local line = proto.lineDefined
  local basepc = 0
  for _, ai in ipairs(proto.abslineinfo) do
    if ai.pc <= pc then
      line = ai.line
      basepc = ai.pc
    end
  end
  for i = basepc + 1, pc do
    if i <= #proto.lineinfo then
      line = line + proto.lineinfo[i]
    end
  end
  return line
end

-- ---------------------------------------------------------------------------
-- Format a constant for display
-- ---------------------------------------------------------------------------
local function fmtK(k)
  if k.type == "nil" then return "nil"
  elseif k.type == "false" then return "false"
  elseif k.type == "true" then return "true"
  elseif k.type == "int" then return tostring(k.value)
  elseif k.type == "float" then
    local s = string.format("%.14g", k.value)
    if not string.find(s, "[%.eE]") then s = s .. ".0" end
    return s
  elseif k.type == "string" then return string.format("%q", k.value)
  else return "?" end
end

-- ---------------------------------------------------------------------------
-- Dump a proto to text
-- ---------------------------------------------------------------------------
local function dumpProto(proto, indent)
  indent = indent or ""
  local out = {}
  local function emit(s) out[#out+1] = s end

  emit(string.format("%sfunction <%s:%d,%d> (%d instructions)",
    indent, proto.source or "?", proto.lineDefined, proto.lastLine, #proto.code))
  emit(string.format("%s%d params, %d slots, %d upvalues, %d locals, %d constants, %d functions",
    indent, proto.numParams, proto.maxStack, #proto.upvalues,
    #proto.locvars, #proto.constants, #proto.protos))

  for pc = 1, #proto.code do
    local inst = proto.code[pc]
    local op = getOP(inst)
    local name = opnames[op] or ("OP_" .. op)
    local mode = opmodes[op] or 0
    local line = getLine(proto, pc)
    local args

    if mode == 0 then  -- iABC
      local a, b, c, k = getA(inst), getB(inst), getC(inst), getK(inst)
      if k ~= 0 then
        args = string.format("%d %d %d ; k=1", a, b, c)
      else
        args = string.format("%d %d %d", a, b, c)
      end
    elseif mode == 1 then  -- ivABC
      local a, vb, vc, k = getA(inst), getVB(inst), getVC(inst), getK(inst)
      if k ~= 0 then
        args = string.format("%d %d %d ; k=1", a, vb, vc)
      else
        args = string.format("%d %d %d", a, vb, vc)
      end
    elseif mode == 2 then  -- iABx
      args = string.format("%d %d", getA(inst), getBx(inst))
    elseif mode == 3 then  -- iAsBx
      args = string.format("%d %d", getA(inst), getsBx(inst))
    elseif mode == 4 then  -- iAx
      args = string.format("%d", getAx(inst))
    elseif mode == 5 then  -- isJ
      args = string.format("%d", getsJ(inst))
    end

    emit(string.format("%s\t%d\t[%d]\t%-12s\t%s", indent, pc, line, name, args))
  end

  emit(string.format("%sconstants (%d):", indent, #proto.constants))
  for i = 1, #proto.constants do
    emit(string.format("%s\t%d\t%s", indent, i-1, fmtK(proto.constants[i])))
  end

  emit(string.format("%slocals (%d):", indent, #proto.locvars))
  for i = 1, #proto.locvars do
    local lv = proto.locvars[i]
    emit(string.format("%s\t%d\t%s\t%d\t%d", indent, i-1, lv.name or "?", lv.startpc, lv.endpc))
  end

  emit(string.format("%supvalues (%d):", indent, #proto.upvalues))
  for i = 1, #proto.upvalues do
    local uv = proto.upvalues[i]
    emit(string.format("%s\t%d\t%s\t%d\t%d\t%d", indent, i-1,
      uv.name or "?", uv.instack, uv.idx, uv.kind))
  end

  for i = 1, #proto.protos do
    emit("")
    emit(dumpProto(proto.protos[i], indent))
  end

  return table.concat(out, "\n")
end

-- ---------------------------------------------------------------------------
-- Main
-- ---------------------------------------------------------------------------
local function main()
  local source
  if arg[1] == "-" then
    source = io.read("*a")
  elseif arg[1] then
    local f = assert(io.open(arg[1]))
    source = f:read("*a")
    f:close()
  else
    io.stderr:write("Usage: lua disasm.lua file.lua | lua disasm.lua -\n")
    os.exit(1)
  end

  local func = assert(load(source, "=input"))
  local bytecode = string.dump(func)
  local r = Reader.new(bytecode)

  -- Parse header (see lundump.c checkHeader):
  -- 4 bytes: LUA_SIGNATURE "\x1bLua"
  -- 1 byte: LUAC_VERSION
  -- 1 byte: LUAC_FORMAT
  -- 6 bytes: LUAC_DATA "\x19\x93\r\n\x1a\n"
  -- Then 4 checknum entries: int, Instruction, lua_Integer, lua_Number
  --   each: 1 byte size + <size> bytes sample value
  -- 1 byte: number of upvalues for main function
  r.pos = 1 + 4 + 1 + 1 + 6  -- skip signature + version + format + LUAC_DATA
  for _ = 1, 4 do
    local sz = r:byte()
    r.pos = r.pos + sz
  end
  r:byte()  -- number of upvalues

  -- Read main proto
  local proto = readProto(r, "=input")
  print(dumpProto(proto))
end

main()