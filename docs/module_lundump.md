# lundump 模块规格书

## 模块职责

反序列化 Lua 字节码文件（.luac）。包括常量表、函数原型、调试信息的二进制格式解析。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lzio | 输入流 |
| lfunc | Proto, Closure |
| lstring | 字符串 |
| lstate | 状态 |

## 公开 API

```c
/* 加载字节码 */
LUAI_FUNC LClosure *luaU_undump (lua_State *L, ZIO *z, 
                                  const char *name, int mode);

/* 字节码格式常量 */
#define LUAC_VERSION       0x53   /* Lua 5.5 */
#define LUAC_FORMAT        0     /* official */
#define LUAC_DATA          "\x19\x93\r\n\x1a\n"
#define LUAC_INT           0
#define LUAC_NUM           0

/* 头大小 */
#define LUAI_HEADSIZE        12
#define LUAI_HEADSIZE_LUA50  12

/* 字节码写入 */
LUAI_FUNC int luaU_dump (lua_State *L, const Proto *f, 
                          lua_Writer w, void *data, int strip);
```

## 二进制格式

```
┌─────────────────────────────────────────┐
│  Header                                 │
├─────────────────────────────────────────┤
│  string table                           │
├─────────────────────────────────────────┤
│  functions (protos)                      │
├─────────────────────────────────────────┤
│  main function                          │
└─────────────────────────────────────────┘
```

### Header 格式

```
byte    signature[4]    = "\x1bLua"        /* Lua 签名 */
byte    version         = 0x53            /* 5.3 */
byte    format          = 0                /* official */
byte    luac_data[6]   = "\x19\x93\r\n\x1a\n"
byte    cint_size       = sizeof(int)
byte    size_t          = sizeof(size_t)
byte    instruction_size = sizeof(Instruction)
byte    lua_Integer_size = sizeof(lua_Integer)
byte    lua_Number_size = sizeof(lua_Number)
byte    integral        = (lua_Number)0 == (lua_Integer)0
```

## Go 重写规格

```go
package lua

// DumpState 字节码加载器状态
type DumpState struct {
    L       *LuaState
    Z       *ZIO
    Status  int
    Name    string
    
    // 函数原型的加载上下文
    Source  *TString
    PC      int  // 当前指令位置
}

// luaU_undump: 加载字节码
func (L *LuaState) Undump(z *ZIO, name string, mode int) *LClosure {
    d := &DumpState{
        L:    L,
        Z:    z,
        Name: name,
    }
    
    // 读取并验证 header
    d.loadHeader()
    
    // 读取字符串表
    strings := d.loadStringTable()
    
    // 读取主函数
    f := d.loadFunction(nil)
    
    // 创建闭包
    cl := L.NewLClosure(f)
    
    // 设置 _ENV
    // ...
    
    return cl
}

// loadHeader
func (d *DumpState) loadHeader() {
    // 验证签名
    sig := [4]byte{0x1B, 'L', 'u', 'a'}
    for i := 0; i < 4; i++ {
        if d.readByte() != sig[i] {
            d.Error("not a valid Lua binary")
        }
    }
    
    // 验证版本
    // ⚠️ Lua 5.5: LUAC_VERSION = 0x55 = 85
    version := d.readByte()
    if version != 0x55 {
        d.Error("version mismatch: expected Lua 5.5 (0x55)")
    }
    
    // 忽略 format, data, sizes
    d.Z.ReadBytes(6)  // luac_data
    d.readByte()      // cint_size
    d.readByte()      // size_t
    d.readByte()      // instruction_size
    d.readByte()      // lua_Integer_size
    d.readByte()      // lua_Number_size
    d.readByte()      // integral
}

// loadFunction
func (d *DumpState) loadFunction(parent *Proto) *Proto {
    p := &Proto{
        Source: d.Source,
    }
    
    // 行号信息
    p.LineDefined = d.readInt()
    p.LastLineDefined = d.readInt()
    
    // 参数和栈
    p.NumParams = d.readByte()
    p.Flag = d.readByte()
    p.MaxStackSize = d.readByte()
    
    // 常量数
    nk := d.readInt()
    p.K = make([]TValue, nk)
    for i := 0; i < nk; i++ {
        p.K[i] = d.loadConstant()
    }
    
    // 函数数
    np := d.readInt()
    p.P = make([]*Proto, np)
    for i := 0; i < np; i++ {
        p.P[i] = d.loadFunction(p)
    }
    
    // 指令数
    ns := d.readInt()
    p.Code = make([]Instruction, ns)
    for i := 0; i < ns; i++ {
        p.Code[i] = d.loadInstruction()
    }
    
    // upvalue
    nu := d.readInt()
    p.Upvalues = make([]UpvalDesc, nu)
    for i := 0; i < nu; i++ {
        p.Upvalues[i].Name = d.loadString()
        p.Upvalues[i].InStack = d.readByte()
        p.Upvalues[i].Idx = d.readByte()
        p.Upvalues[i].Kind = d.readByte()
    }
    
    // 调试信息（可选）
    if !d.L.IStripping {
        d.loadDebugInfo(p)
    }
    
    return p
}

// loadConstant
func (d *DumpState) loadConstant() TValue {
    t := d.readByte()
    switch t {
    case LUA_TNIL:
        return MakeNil()
    case LUA_TBOOLEAN:
        return MakeBoolean(d.readByte() != 0)
    case LUA_TNUMBER:
        return MakeNumber(d.readNumber())
    case LUA_TSTRING:
        s := d.loadString()
        return MakeString(s)
    case LUA_TINT:
        return MakeInteger(int64(d.readInt64()))
    default:
        d.Error("unknown constant type")
        return TValue{}
    }
}

// loadInstruction
func (d *DumpState) loadInstruction() Instruction {
    // 大端序读取 4 字节
    var ins Instruction
    ins = Instruction(d.readByte())
    ins |= Instruction(d.readByte()) << 8
    ins |= Instruction(d.readByte()) << 16
    ins |= Instruction(d.readByte()) << 24
    return ins
}

// loadString
func (d *DumpState) loadString() *TString {
    size := d.readSizeT()
    if size == 0 {
        return nil
    }
    data := d.Z.ReadBytes(int(size) - 1)
    d.Z.ReadByte()  // 跳过 '\0'
    return d.L.NewString(string(data))
}

// loadDebugInfo
func (d *DumpState) loadDebugInfo(p *Proto) {
    // 行号信息
    nlineinfo := d.readInt()
    p.LineInfo = make([]int8, nlineinfo)
    for i := 0; i < nlineinfo; i++ {
        p.LineInfo[i] = int8(d.readByte())
    }
    
    // 绝对行号
    nabslineinfo := d.readInt()
    p.AbsLineInfo = make([]AbsLineInfo, nabslineinfo)
    for i := 0; i < nabslineinfo; i++ {
        p.AbsLineInfo[i].PC = d.readInt()
        p.AbsLineInfo[i].Line = d.readInt()
    }
    
    // 局部变量
    nlocvars := d.readInt()
    p.LocVars = make([]LocVar, nlocvars)
    for i := 0; i < nlocvars; i++ {
        p.LocVars[i].Varname = d.loadString()
        p.LocVars[i].StartPC = d.readInt()
        p.LocVars[i].EndPC = d.readInt()
    }
    
    // upvalue 名称
    for i := 0; i < len(p.Upvalues); i++ {
        if p.Upvalues[i].Name == nil {
            p.Upvalues[i].Name = d.loadString()
        }
    }
}
```

## 字节码验证

```go
// luaU_headerPosfix 验证 header
func (d *DumpState) validateHeader() {
    // 大小端检测
    test := uint32(0x01020304)
    bytes := (*[4]byte)(unsafe.Pointer(&test))[:]
    
    expected := d.Z.ReadBytes(4)
    if bytes[0] != expected[0] {
        d.Error("big-endian bytecode not supported")
    }
}
```

## 陷阱和注意事项

### 陷阱 1: 大小端

```go
// Lua 字节码可以是 big-endian 或 little-endian
// 必须在 header 中检测
```

### 陷阱 2: 整数类型

```go
// Lua 5.5+ 支持在字节码中嵌入整数
// 需要检测 LUA_INT
```

### 陷阱 3: 调试信息

```go
// strip 模式下不加载调试信息
// 但 upvalue 名称仍然需要（除非也 strip）
```

## 验证测试

```lua
-- 生成字节码
luac -o test.luac test.lua

-- 检查字节码结构
local f = loadfile("test.luac")
print(f)  -- function

-- 反编译
luac -p test.luac
```