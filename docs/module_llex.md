# llex 模块规格书

## 模块职责

词法分析器（Scanner）。将 Lua 源代码文本转换为词符流（Token Stream），供语法分析器使用。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lzio | 输入流抽象 |
| lobject | Token 定义 |
| lstate | 状态管理 |

## 公开 API

```c
/* 词符 */
typedef union Token {
  lua_Number r;
  TString *ts;
  int t;
} Token;

typedef struct LexState {
  int current;          /* 当前字符 */
  int linenumber;       /* 当前行号 */
  int lastline;        /* 上一词符所在行 */
  Token t;             /* 当前词符 */
  Token lookahead;     /* 下一个词符 */
  struct FuncState *fs; /* 指向外层函数状态 */
  lua_State *L;
  ZIO *z;             /* 输入 */
  Mbuffer *buff;       /* 词符缓冲区 */
  int nestlevel;      /* 嵌套层级 */
  struct Dyndata *dyd; /* 动态数据 */
  TString *source;    /* 源码标识 */
  TString *envn;     /* _ENV 名称 */
} LexState;

/* 词符类型 */
#define TK_AND         257
#define TK_BREAK       258
#define TK_DO          259
#define TK_ELSE        260
#define TK_ELSEIF      261
#define TK_END         262
#define TK_FALSE       263
#define TK_FOR         264
#define TK_FUNCTION    265
#define TK_GOTO        266
#define TK_IF          267
#define TK_IN          268
#define TK_LOCAL       269
#define TK_NIL         270
#define TK_NOT         271
#define TK_OR          272
#define TK_RETURN      273
#define TK_THEN        274
#define TK_TRUE        275
#define TK_UNTIL       276
#define TK_WHILE       277
#define TK_ID          278
#define TK_STRING      279
#define TK_NUMBER      280
#define TK_OP          281
#define TK_EOS         282

LUAI_FUNC void luaX_init (lua_State *L);
LUAI_FUNC void luaX_setinput (lua_State *L, LexState *ls, ZIO *z, 
                               TString *source, int firstchar);
LUAI_FUNC TString *luaX_newstring (LexState *ls, const char *str, size_t l);
LUAI_FUNC void luaX_next (LexState *ls);
LUAI_FUNC int luaX_lookahead (LexState *ls);
LUAI_FUNC l_noret luaX_syntaxerror (LexState *ls, const char *msg);
LUAI_FUNC const char *luaX_token2str (LexState *ls, int token);
```

## 词符类型

```go
package lua

// TokenType 词符类型
type TokenType int

const (
    // 终结符
    TK_AND    TokenType = 257 + iota
    TK_BREAK
    TK_DO
    TK_ELSE
    TK_ELSEIF
    TK_END
    TK_FALSE
    TK_FOR
    TK_FUNCTION
    TK_GOTO
    TK_IF
    TK_IN
    TK_LOCAL
    TK_NIL
    TK_NOT
    TK_OR
    TK_RETURN
    TK_THEN
    TK_TRUE
    TK_UNTIL
    TK_WHILE
    
    // 终结符（动态）
    TK_ID       // 标识符
    TK_STRING   // 字符串
    TK_NUMBER   // 数值
    TK_EOS      // 文件结束
)

// Token 词符
type Token struct {
    Type  TokenType
    Val   TValue  // 数值（用于 NUMBER, STRING）
    Str   *TString  // 字符串值
}
```

## LexState 结构

```go
// LexState 词法分析器状态
type LexState struct {
    L         *LuaState
    Z         *ZIO         // 输入流
    Source    *TString      // 源码标识
    Envn     *TString      // _ENV 名称
    
    Current   int           // 当前字符
    Linenumber int         // 当前行号
    Lastline   int          // 上一词符行号
    
    T         Token        // 当前词符
    Lookahead  Token       // 下一个词符（用于 lookahead）
    
    NestLevel  int          // 嵌套层级
    
    Buff       *Mbuffer     // 缓冲区
    Dyd        *Dyndata     // 动态数据（goto 标签）
    
    FS         *FuncState   // 外层函数状态
}

// Mbuffer 缓冲区
type Mbuffer struct {
    Buffer []byte
    Len    int
}
```

## Go 重写规格

### 词法分析器

```go
// luaX_setinput
func (ls *LexState) SetInput(L *LuaState, z *ZIO, source string) {
    ls.L = L
    ls.Z = z
    ls.Source = L.NewString(source)
    ls.Envn = L.NewString("_ENV")
    ls.Linenumber = 1
    ls.NestLevel = 0
    ls.Buff = &Mbuffer{make([]byte, 0, 64), 0}
    
    // 读取第一个字符
    ls.Next()
}

// luaX_next: 获取下一个词符
func (ls *LexState) Next() {
    ls.T = ls.Lookahead
    ls.Lookahead = ls.scan()
}

// luaX_lookahead: 查看下一个词符
func (ls *LexState) Lookahead() TokenType {
    if ls.Lookahead.Type == 0 {
        ls.Lookahead = ls.scan()
    }
    return ls.Lookahead.Type
}

// scan: 扫描单个词符
func (ls *LexState) scan() Token {
    ls.skipWhitespace()
    
    switch c := ls.Current; {
    case -1:
        return Token{Type: TK_EOS}
    
    case 'a', 'b', ..., 'z', 'A', 'B', ..., 'Z', '_':
        return ls.scanIdentifier()
    
    case '0', '1', ..., '9':
        return ls.scanNumber()
    
    case '"', '\'':
        return ls.scanString()
    
    case '[':
        return ls.scanLongString()
    
    case '+':
        return Token{Type: '+'}
    
    case '-':
        if ls.Peek() == '-' {
            ls.Next()
            ls.skipComment()
            return ls.scan()  // 继续扫描
        }
        return Token{Type: '-'}
    
    case '*', '/', '%', '^', '#', '(', ')', '{', '}', '[', ']', ';', ':', ',':
        return ls.scanOperator()
    
    case '<':
        return ls.scanComparison("<", "<=", TK_LT, TK_LE)
    
    case '>':
        return ls.scanComparison(">", ">=", TK_GT, TK_GE)
    
    case '=':
        if ls.Peek() == '=' {
            ls.Next()
            return Token{Type: TK_EQ}
        }
        return Token{Type: '='}
    
    case '~':
        if ls.Peek() == '=' {
            ls.Next()
            return Token{Type: TK_NE}
        }
        return Token{Type: '~'}
    
    case '.':
        if ls.Peek() == '.' {
            ls.Next()
            if ls.Peek() == '.' {
                ls.Next()
                return Token{Type: TK_DOTS}
            }
            return Token{Type: TK_CONCAT}
        }
        if isdigit(ls.Peek()) {
            return ls.scanNumber()
        }
        return Token{Type: '.'}
    
    default:
        ls.Error("unexpected symbol")
        return Token{}
    }
}
```

### 标识符和关键字

```go
// 关键字表
var keywords = map[string]TokenType{
    "and":      TK_AND,
    "break":    TK_BREAK,
    "do":       TK_DO,
    "else":     TK_ELSE,
    "elseif":   TK_ELSEIF,
    "end":      TK_END,
    "false":    TK_FALSE,
    "for":      TK_FOR,
    "function": TK_FUNCTION,
    "goto":     TK_GOTO,
    "if":       TK_IF,
    "in":       TK_IN,
    "local":    TK_LOCAL,
    "nil":      TK_NIL,
    "not":      TK_NOT,
    "or":       TK_OR,
    "return":   TK_RETURN,
    "then":     TK_THEN,
    "true":     TK_TRUE,
    "until":    TK_UNTIL,
    "while":    TK_WHILE,
}

func (ls *LexState) scanIdentifier() Token {
    ls.buff.Reset()
    
    for isAlphanumeric(ls.Current) || ls.Current == '_' {
        ls.buff.WriteByte(byte(ls.Current))
        ls.Next()
    }
    
    str := ls.newString(ls.buff.String())
    
    // 检查是否是关键字
    if tok, ok := keywords[str]; ok {
        return Token{Type: tok}
    }
    
    return Token{Type: TK_ID, Str: str}
}
```

### 数值

```go
func (ls *LexState) scanNumber() Token {
    ls.buff.Reset()
    var isFloat = false
    
    for isdigit(ls.Current) || ls.Current == '.' {
        if ls.Current == '.' {
            if isFloat {
                break
            }
            isFloat = true
        }
        ls.buff.WriteByte(byte(ls.Current))
        ls.Next()
    }
    
    // 指数部分
    if ls.Current == 'e' || ls.Current == 'E' {
        isFloat = true
        ls.buff.WriteByte(byte(ls.Current))
        ls.Next()
        if ls.Current == '+' || ls.Current == '-' {
            ls.buff.WriteByte(byte(ls.Current))
            ls.Next()
        }
        for isdigit(ls.Current) {
            ls.buff.WriteByte(byte(ls.Current))
            ls.Next()
        }
    }
    
    // 十六进制
    if ls.Current == 'x' || ls.Current == 'X' {
        // 处理十六进制
    }
    
    // 解析数值
    val := parseNumber(ls.buff.String(), isFloat)
    return Token{Type: TK_NUMBER, Val: MakeNumber(val)}
}
```

### 字符串

```go
func (ls *LexState) scanString() Token {
    quote := ls.Current
    ls.Next()  // 跳过引号
    
    ls.buff.Reset()
    
    for ls.Current != quote && ls.Current != -1 {
        if ls.Current == '\\' {
            ls.Next()
            c := ls.escapeChar()
            ls.buff.WriteByte(c)
        } else if ls.Current == '\n' {
            ls.Error("unfinished string")
        } else {
            ls.buff.WriteByte(byte(ls.Current))
            ls.Next()
        }
    }
    
    ls.Next()  // 跳过结束引号
    
    str := ls.newString(ls.buff.String())
    return Token{Type: TK_STRING, Str: str}
}

func (ls *LexState) escapeChar() byte {
    ls.Next()
    switch ls.Current {
    case 'n': return '\n'
    case 'r': return '\r'
    case 't': return '\t'
    case '\\': return '\\'
    case '"': return '"'
    case '\'': return '\''
    case 'u': return ls.readUnicode()
    case 'z': 
        ls.skipWhitespace()
        return 0
    default:
        if isdigit(ls.Current) {
            return ls.readOctal()
        }
        return byte(ls.Current)
    }
}
```

### 长字符串 [[...]]

```go
func (ls *LexState) scanLongString() Token {
    // 检查 [[ 或 [=[ 等
    level := 0
    if ls.Peek() == '[' {
        saved := ls.PeekN(2)
        level = countEquals(saved)
        ls.Next()  // skip '['
    }
    
    // 跳过第一个 '['
    ls.Next()
    if ls.Current == '\n' {
        ls.Next()
    }
    
    ls.buff.Reset()
    
    for {
        switch ls.Current {
        case -1:
            ls.Error("unfinished long string")
        
        case ']':
            if ls.PeekN(level+1) == ']' {
                // 结束
                ls.NextN(level + 2)
                break
            }
            ls.buff.WriteByte(']')
            ls.Next()
        
        default:
            ls.buff.WriteByte(byte(ls.Current))
            ls.Next()
        }
    }
    
    str := ls.newString(ls.buff.String())
    return Token{Type: TK_STRING, Str: str}
}
```

## 陷阱和注意事项

### 陷阱 1: 长字符串级别匹配

```go
// Lua 5.2+ 支持 [=[...] 等长字符串
// 级别必须匹配
// [=[[...]]=] 有效
// [=[[...]]] 无效
```

### 陷阱 2: 逃逸序列

```lua
-- 各种逃逸序列
"\n" -- 换行
"\r" -- 回车
"\t" -- tab
"\\" -- 反斜杠
"\123" -- 八进制 (最多3位)
"\xAB" -- 十六进制
"\u{1F600}" -- Unicode
"\z" -- 跳过空白
```

### 陷阱 3: UTF-8

Lua 5.3+ 标识符支持 UTF-8（取决于 locale）。Go 实现需要正确处理 rune。

## 验证测试

```lua
-- 基本词符
local keywords = {"and", "break", "do", "else", "end", "false", 
                 "for", "function", "if", "in", "local", "nil", 
                 "not", "or", "return", "then", "true", "until", "while"}

-- 字符串
local s1 = "hello world"
local s2 = 'single quotes'
local s3 = [[long string]]
local s4 = [=[with level]=]

-- 数值
local i = 42
local f = 3.14
local hex = 0xFF
local exp = 1e10

-- 注释
-- 单行注释
--[[
    多行注释
]]
```

## 与 lparser 的交互

```go
// lparser 调用词法分析器
func (ls *LexState) next() {
    ls.Next()
    ls.T = ls.Lookahead
    ls.Lookahead.Type = 0  // 重置 lookahead
}

func (ls *LexState) lookahead() TokenType {
    return ls.Lookahead().Type
}

// 检查并消费词符
func (ls *LexState) check(t TokenType) {
    if ls.T.Type != t {
        ls.Error("expected %s, got %s", t, ls.T.Type)
    }
}

func (ls *LexState) checknext(t TokenType) {
    ls.check(t)
    ls.next()
}
```