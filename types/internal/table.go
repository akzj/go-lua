// Package internal provides concrete implementations of api interfaces.
package internal

import (
	"github.com/akzj/go-lua/types/api"
)

// GCObject is the common header for all collectable objects.
type GCObject struct {
	Next   *GCObject
	Tt     uint8
	Marked uint8
}

func (g *GCObject) IsTable() bool {
	return int(g.Tt) == api.Ctb(int(api.LUA_VTABLE))
}

// Table is the concrete Table implementation.
type Table struct {
	GCObject
	Flags     uint8
	Lsizenode uint8
	Asize     uint32
	Array     []*TValue
	Node      *Node
	Metatable *Table
	GClist    *GCObject
}

func (t *Table) SizeNode() int {
	if t.Lsizenode >= 32 {
		return 0
	}
	return 1 << t.Lsizenode
}

func NewTable() api.Table {
	return &Table{
		GCObject:  GCObject{Tt: uint8(api.Ctb(int(api.LUA_VTABLE)))},
		Flags:     0,
		Lsizenode: 0,
		Asize:     0,
		Array:     make([]*TValue, 0),
	}
}

// Node is the concrete Node implementation.
type Node struct {
	KeyValue Value
	KeyTt    uint8
	KeyNext  int32
	Val      TValue
}

func (n *Node) KeyIsNil() bool         { return n.KeyTt == api.LUA_TNIL }
func (n *Node) KeyIsDead() bool        { return n.KeyTt == api.LUA_TDEADKEY }
func (n *Node) KeyIsCollectable() bool { return int(n.KeyTt)&api.BIT_ISCOLLECTABLE != 0 }
