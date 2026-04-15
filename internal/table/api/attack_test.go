package api

import (
	"math"
	"testing"

	obj "github.com/akzj/go-lua/internal/object/api"
)

// TestAttackTableLength is a regression test for the "attack on table length"
// pattern from nextvar.lua. A table with keys at every power of 2 must return
// a small boundary, not 2^62 (which would hang "for i=1,#t do end").
func TestAttackTableLength(t *testing.T) {
	lim := int(math.Floor(math.Log2(float64(math.MaxInt64)))) - 1
	tbl := New(0, lim+1)
	for i := lim; i >= 0; i-- {
		tbl.SetInt(int64(1)<<i, obj.TValue{Tt: obj.TagTrue})
	}
	n := tbl.RawLen()
	// Valid boundaries: 2 (t[1]=true, t[2]=true, t[3]=nil).
	// Any value > asize is wrong — the loop would run billions of iterations.
	if n > int64(len(tbl.Array))+64 {
		t.Fatalf("RawLen() = %d; want small boundary (got huge value that would hang)", n)
	}
}
