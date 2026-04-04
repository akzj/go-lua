// Package internal provides concrete implementations of AST nodes.
package internal

import (
	"github.com/akzj/go-lua/ast/api"
	lexapi "github.com/akzj/go-lua/lex/api"
	typesapi "github.com/akzj/go-lua/types/api"
)

// =============================================================================
// Base Node
// =============================================================================

// BaseNode provides common position tracking for all nodes.
type BaseNode struct {
	Line   int
	Column int
}

func (b *BaseNode) Position() (int, int) {
	return b.Line, b.Column
}

// =============================================================================
// Expression Nodes
// =============================================================================

// NilExp represents nil constant.
type NilExp struct {
	BaseNode
}

func (e *NilExp) IsConstant() bool { return true }

// TrueExp represents true constant.
type TrueExp struct {
	BaseNode
}

func (e *TrueExp) IsConstant() bool { return true }

// FalseExp represents false constant.
type FalseExp struct {
	BaseNode
}

func (e *FalseExp) IsConstant() bool { return true }

// IntegerExp represents integer literal.
type IntegerExp struct {
	BaseNode
	Value int64
}

func (e *IntegerExp) IsConstant() bool { return true }
func (e *IntegerExp) Kind() api.ExpKind { return api.EXP_KINTEGER }
func (e *IntegerExp) Position() (int, int) { return e.BaseNode.Line, e.BaseNode.Column }

// FloatExp represents floating-point literal.
type FloatExp struct {
	BaseNode
	Value float64
}

func (e *FloatExp) IsConstant() bool { return true }

// StringExp represents string literal.
type StringExp struct {
	BaseNode
	Value string
}

func (e *StringExp) IsConstant() bool { return true }
func (e *StringExp) Kind() api.ExpKind { return api.EXP_KSTRING }
func (e *StringExp) Position() (int, int) { return e.BaseNode.Line, e.BaseNode.Column }

// VarargExp represents vararg expression (...).
type VarargExp struct {
	BaseNode
}

func (e *VarargExp) IsConstant() bool { return false }

// NameExp represents a variable name (identifier).
type NameExp struct {
	BaseNode
	Name string
}

func (e *NameExp) IsConstant() bool { return false }
func (e *NameExp) Kind() api.ExpKind { return api.EXP_LOCAL }
func (e *NameExp) Position() (int, int) { return e.BaseNode.Line, e.BaseNode.Column }

// GetName implements the nameAccess interface for global variable access
func (e *NameExp) GetName() string { return e.Name }

// =============================================================================
// Expression Descriptor (for parser use)
// =============================================================================

// ExpDescImpl is the concrete implementation of ExpDesc.
type ExpDescImpl struct {
	BaseNode
	Kind_        api.ExpKind
	Reg_         int
	Info_        int
	TableReg_    int
	KeyReg_      int
	KeyIsString_ bool
	TrueJump_    int
	FalseJump_   int
}

func (e *ExpDescImpl) Kind() api.ExpKind         { return e.Kind_ }
func (e *ExpDescImpl) SetKind(k api.ExpKind)     { e.Kind_ = k }
func (e *ExpDescImpl) Reg() int                  { return e.Reg_ }
func (e *ExpDescImpl) SetReg(r int)              { e.Reg_ = r }
func (e *ExpDescImpl) Info() int                 { return e.Info_ }
func (e *ExpDescImpl) SetInfo(i int)             { e.Info_ = i }
func (e *ExpDescImpl) Table() (int, int)         { return e.TableReg_, e.KeyReg_ }
func (e *ExpDescImpl) SetTable(t, k int)         { e.TableReg_, e.KeyReg_ = t, k }
func (e *ExpDescImpl) KeyIsString() bool         { return e.KeyIsString_ }
func (e *ExpDescImpl) SetKeyIsString(b bool)     { e.KeyIsString_ = b }
func (e *ExpDescImpl) TrueJump() int             { return e.TrueJump_ }
func (e *ExpDescImpl) SetTrueJump(j int)         { e.TrueJump_ = j }
func (e *ExpDescImpl) FalseJump() int            { return e.FalseJump_ }
func (e *ExpDescImpl) SetFalseJump(j int)        { e.FalseJump_ = j }
func (e *ExpDescImpl) IsConstant() bool           { return false }

// =============================================================================
// Binary/Unary Expression
// =============================================================================

// BinopExp represents binary operation.
type BinopExp struct {
	BaseNode
	Op    api.BinopKind
	Left  api.ExpNode
	Right api.ExpNode
}

func (e *BinopExp) IsConstant() bool {
	return e.Left.IsConstant() && e.Right.IsConstant()
}
func (e *BinopExp) Kind() api.ExpKind { return api.EXP_RELOC }
func (e *BinopExp) Position() (int, int) { return e.BaseNode.Line, e.BaseNode.Column }

// UnopExp represents unary operation.
type UnopExp struct {
	BaseNode
	Op   api.UnopKind
	Exp  api.ExpNode
}

func (e *UnopExp) IsConstant() bool {
	return e.Exp.IsConstant()
}
func (e *UnopExp) Kind() api.ExpKind { return api.EXP_RELOC }
func (e *UnopExp) Position() (int, int) { return e.BaseNode.Line, e.BaseNode.Column }

// =============================================================================
// Table Constructor
// =============================================================================

// TableConstructorImpl is the concrete implementation.
type TableConstructorImpl struct {
	BaseNode
	ArrayFields []api.ExpNode
	RecordFields []struct {
		Key   api.ExpNode
		Value api.ExpNode
	}
}

func (t *TableConstructorImpl) NumFields() int   { return len(t.ArrayFields) }
func (t *TableConstructorImpl) NumRecords() int   { return len(t.RecordFields) }
func (t *TableConstructorImpl) AddArrayField(e api.ExpNode) { t.ArrayFields = append(t.ArrayFields, e) }
func (t *TableConstructorImpl) AddRecordField(k, v api.ExpNode) {
	t.RecordFields = append(t.RecordFields, struct{ Key, Value api.ExpNode }{k, v})
}
func (t *TableConstructorImpl) IsConstant() bool {
	for _, f := range t.ArrayFields {
		if !f.IsConstant() {
			return false
		}
	}
	for _, f := range t.RecordFields {
		if !f.Key.IsConstant() || !f.Value.IsConstant() {
			return false
		}
	}
	return true
}
func (t *TableConstructorImpl) Kind() api.ExpKind { return api.EXP_RELOC }
func (t *TableConstructorImpl) Position() (int, int) { return t.BaseNode.Line, t.BaseNode.Column }

// =============================================================================
// Function Call
// =============================================================================

// FuncCallImpl is the concrete implementation.
type FuncCallImpl struct {
	BaseNode
	Func_       api.ExpNode
	Args_       []api.ExpNode
	NumResults_ int
}

func (f *FuncCallImpl) Func() api.ExpNode         { return f.Func_ }
func (f *FuncCallImpl) Args() []api.ExpNode       { return f.Args_ }
func (f *FuncCallImpl) NumResults() int           { return f.NumResults_ }
func (f *FuncCallImpl) IsConstant() bool         { return false }
func (f *FuncCallImpl) Kind() api.ExpKind        { return api.EXP_CALL }
func (f *FuncCallImpl) Position() (int, int)    { return f.BaseNode.Line, f.BaseNode.Column }

// =============================================================================
// Function Definition
// =============================================================================

// FuncDefImpl is the concrete implementation.
type FuncDefImpl struct {
	Line_       int
	LastLine_   int
	IsLocal_    bool
	Proto_      *typesapi.Proto
}

func (f *FuncDefImpl) IsLocal() bool         { return f.IsLocal_ }
func (f *FuncDefImpl) Line() int             { return f.Line_ }
func (f *FuncDefImpl) LastLine() int        { return f.LastLine_ }
func (f *FuncDefImpl) Proto() *typesapi.Proto   { return f.Proto_ }

// =============================================================================
// Statement Nodes
// =============================================================================

// AssignStat represents assignment statement.
type AssignStat struct {
	BaseNode
	Vars []api.ExpNode
	Exprs []api.ExpNode
}

func (s *AssignStat) IsScopeEnd() bool { return false }

// LocalVarStat represents local variable declaration.
type LocalVarStat struct {
	BaseNode
	Names []string
	Exprs []api.ExpNode
}

func (s *LocalVarStat) IsScopeEnd() bool { return false }

// LocalFuncStat represents local function declaration.
type LocalFuncStat struct {
	BaseNode
	Name string
	Func api.FuncDef
}

func (s *LocalFuncStat) IsScopeEnd() bool { return false }

// GlobalFuncStat represents global function declaration.
type GlobalFuncStat struct {
	BaseNode
	Name string
	Func api.FuncDef
}

func (s *GlobalFuncStat) IsScopeEnd() bool { return true }

// IfStat represents if/elseif/else statement.
type IfStat struct {
	BaseNode
	Conds []api.ExpNode
	Blocks []*BlockImpl
}

func (s *IfStat) IsScopeEnd() bool { return true }

// WhileStat represents while loop.
type WhileStat struct {
	BaseNode
	Cond api.ExpNode
	Block *BlockImpl
}

func (s *WhileStat) IsScopeEnd() bool { return false }

// RepeatStat represents repeat until loop.
type RepeatStat struct {
	BaseNode
	Block *BlockImpl
	Cond api.ExpNode
}

func (s *RepeatStat) IsScopeEnd() bool { return false }

// ForInStat represents for-in loop.
type ForInStat struct {
	BaseNode
	Names []string
	Exprs []api.ExpNode
	Block *BlockImpl
}

func (s *ForInStat) IsScopeEnd() bool { return false }

// ForNumStat represents numeric for loop.
type ForNumStat struct {
	BaseNode
	Var  string
	Init api.ExpNode
	Limit api.ExpNode
	Step api.ExpNode
	Block *BlockImpl
}

func (s *ForNumStat) IsScopeEnd() bool { return false }

// ReturnStat represents return statement.
type ReturnStat struct {
	BaseNode
	Exprs []api.ExpNode
}

func (s *ReturnStat) IsScopeEnd() bool { return true }
func (s *ReturnStat) Kind() api.StatKind { return api.STAT_RETURN }

// BreakStat represents break statement.
type BreakStat struct {
	BaseNode
}

func (s *BreakStat) IsScopeEnd() bool { return true }

// GotoStat represents goto statement.
type GotoStat struct {
	BaseNode
	Name string
}

func (s *GotoStat) IsScopeEnd() bool { return false }

// LabelStat represents label statement.
type LabelStat struct {
	BaseNode
	Name string
}

func (s *LabelStat) IsScopeEnd() bool { return false }

// CallStat represents expression statement (function call).
type CallStat struct {
	BaseNode
	Call api.FuncCall
}

func (s *CallStat) IsScopeEnd() bool { return false }

// EmptyStat represents empty statement (semicolon).
type EmptyStat struct {
	BaseNode
}

func (s *EmptyStat) IsScopeEnd() bool { return false }

// =============================================================================
// Block
// =============================================================================

// BlockImpl is the concrete implementation.
type BlockImpl struct {
	BaseNode
	Stats_    []api.StatNode
	ReturnExp_ []api.ExpNode
}

func (b *BlockImpl) Stats() []api.StatNode     { return b.Stats_ }
func (b *BlockImpl) ReturnExp() []api.ExpNode  { return b.ReturnExp_ }

// =============================================================================
// Chunk
// =============================================================================

// ChunkImpl is the concrete implementation.
type ChunkImpl struct {
	Block_       *BlockImpl
	SourceName_  string
}

func (c *ChunkImpl) Block() *BlockImpl           { return c.Block_ }
func (c *ChunkImpl) SourceName() string          { return c.SourceName_ }

// =============================================================================
// Variable Descriptor
// =============================================================================

// VarDescImpl is the concrete implementation.
type VarDescImpl struct {
	Name_    string
	IsLocal_ bool
	Reg_     int
	Index_   int
}

func (v *VarDescImpl) Name() string    { return v.Name_ }
func (v *VarDescImpl) IsLocal() bool   { return v.IsLocal_ }
func (v *VarDescImpl) IsGlobal() bool  { return !v.IsLocal_ }
func (v *VarDescImpl) Reg() int        { return v.Reg_ }
func (v *VarDescImpl) Index() int      { return v.Index_ }

// =============================================================================
// Label Descriptor
// =============================================================================

// LabelDescImpl is the concrete implementation.
type LabelDescImpl struct {
	Name_ string
	Line_ int
}

func (l *LabelDescImpl) Name() string { return l.Name_ }
func (l *LabelDescImpl) Line() int   { return l.Line_ }

// =============================================================================
// LocalVars
// =============================================================================

// LocalVarsImpl maintains active local variables.
type LocalVarsImpl struct {
	vars []api.VarDesc
}

func (l *LocalVarsImpl) Add(name string, reg int) {
	l.vars = append(l.vars, &VarDescImpl{Name_: name, IsLocal_: true, Reg_: reg, Index_: len(l.vars)})
}

func (l *LocalVarsImpl) Get(index int) api.VarDesc {
	if index >= 0 && index < len(l.vars) {
		return l.vars[index]
	}
	return nil
}

func (l *LocalVarsImpl) Count() int { return len(l.vars) }
func (l *LocalVarsImpl) Reset()     { l.vars = nil }

// =============================================================================
// GotoList
// =============================================================================

// GotoDesc describes a pending goto.
type GotoDesc struct {
	Name_    string
	Pc_      int
	Line_    int
	Nactvar_ int
	Close_   bool
}

func (g *GotoDesc) Name() string { return g.Name_ }
func (g *GotoDesc) Line() int   { return g.Line_ }

// GotoListImpl maintains pending gotos.
type GotoListImpl struct {
	gotos []GotoDesc
}

func (g *GotoListImpl) Add(name string, pc, line, nactvar int, close bool) {
	g.gotos = append(g.gotos, GotoDesc{Name_: name, Pc_: pc, Line_: line, Nactvar_: nactvar, Close_: close})
}

func (g *GotoListImpl) Get(index int) api.LabelDesc {
	if index >= 0 && index < len(g.gotos) {
		return &g.gotos[index]
	}
	return nil
}

func (g *GotoListImpl) Count() int { return len(g.gotos) }

// =============================================================================
// LabelList
// =============================================================================

// LabelListImpl maintains active labels.
type LabelListImpl struct {
	labels []LabelDescImpl
}

func (l *LabelListImpl) Add(name string, pc, line, nactvar int) {
	l.labels = append(l.labels, LabelDescImpl{Name_: name, Line_: line})
}

func (l *LabelListImpl) Get(index int) api.LabelDesc {
	if index >= 0 && index < len(l.labels) {
		return &l.labels[index]
	}
	return nil
}

func (l *LabelListImpl) Count() int { return len(l.labels) }

// =============================================================================
// Token Helper
// =============================================================================

// HasTokenImpl provides token access.
type HasTokenImpl struct {
	Token_ lexapi.Token
}

func (h *HasTokenImpl) Token() lexapi.Token { return h.Token_ }
