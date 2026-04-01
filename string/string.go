// Package string implements Lua string interning via StringTable.
// Root package re-exports the API and imports internal to trigger init().
package string

import (
	"github.com/akzj/go-lua/string/api"
	stringinternal "github.com/akzj/go-lua/string/internal"
)

// Re-export API types and functions for convenience.
type (
	StringTable = api.StringTable
	TStringImpl = api.TStringImpl
)

const MaxShortStringLen = api.MaxShortStringLen

var DefaultStringTable = api.DefaultStringTable

func NewStringTable() api.StringTable {
	return api.NewStringTable()
}

var (
	EqualStrings   = stringinternal.EqualStrings
	IsReservedWord = stringinternal.IsReservedWord
)
