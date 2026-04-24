package stdlib

import (
	"testing"
)

// ===== TABLE FILTER =====

func TestTableFilter(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local evens = table.filter({1,2,3,4,5}, function(v) return v % 2 == 0 end)
		assert(#evens == 2)
		assert(evens[1] == 2)
		assert(evens[2] == 4)
	`)
}

func TestTableFilterEmpty(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.filter({}, function(v) return true end)
		assert(#r == 0)
	`)
}

func TestTableFilterNoneMatch(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.filter({1,2,3}, function(v) return false end)
		assert(#r == 0)
	`)
}

func TestTableFilterAllMatch(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.filter({10,20,30}, function(v) return true end)
		assert(#r == 3)
		assert(r[1] == 10)
		assert(r[2] == 20)
		assert(r[3] == 30)
	`)
}

func TestTableFilterWithIndex(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		-- filter by index: keep only odd-indexed elements
		local r = table.filter({10,20,30,40,50}, function(v, i) return i % 2 == 1 end)
		assert(#r == 3)
		assert(r[1] == 10)
		assert(r[2] == 30)
		assert(r[3] == 50)
	`)
}

func TestTableFilterStrings(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.filter({"apple", "banana", "avocado", "cherry"}, function(v)
			return string.sub(v, 1, 1) == "a"
		end)
		assert(#r == 2)
		assert(r[1] == "apple")
		assert(r[2] == "avocado")
	`)
}

// ===== TABLE MAP =====

func TestTableMap(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local doubled = table.map({1,2,3}, function(v) return v * 2 end)
		assert(#doubled == 3)
		assert(doubled[1] == 2)
		assert(doubled[2] == 4)
		assert(doubled[3] == 6)
	`)
}

func TestTableMapEmpty(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.map({}, function(v) return v end)
		assert(#r == 0)
	`)
}

func TestTableMapToStrings(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.map({1,2,3}, function(v) return tostring(v) end)
		assert(r[1] == "1")
		assert(r[2] == "2")
		assert(r[3] == "3")
	`)
}

func TestTableMapWithIndex(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.map({10,20,30}, function(v, i) return v + i end)
		assert(r[1] == 11) -- 10+1
		assert(r[2] == 22) -- 20+2
		assert(r[3] == 33) -- 30+3
	`)
}

// ===== TABLE REDUCE =====

func TestTableReduce(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local sum = table.reduce({1,2,3,4}, function(acc, v) return acc + v end, 0)
		assert(sum == 10)
	`)
}

func TestTableReduceConcat(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local s = table.reduce({"a","b","c"}, function(acc, v) return acc .. v end, "")
		assert(s == "abc")
	`)
}

func TestTableReduceEmpty(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.reduce({}, function(acc, v) return acc + v end, 42)
		assert(r == 42) -- returns initial value
	`)
}

func TestTableReduceWithIndex(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		-- weighted sum: value * index
		local r = table.reduce({10,20,30}, function(acc, v, i) return acc + v * i end, 0)
		assert(r == 10*1 + 20*2 + 30*3) -- 10 + 40 + 90 = 140
	`)
}

// ===== TABLE KEYS =====

func TestTableKeys(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local ks = table.keys({a=1, b=2, c=3})
		assert(#ks == 3)
		-- keys order is not guaranteed, so check membership
		local found = {}
		for _, k in ipairs(ks) do found[k] = true end
		assert(found["a"])
		assert(found["b"])
		assert(found["c"])
	`)
}

func TestTableKeysArray(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local ks = table.keys({10, 20, 30})
		assert(#ks == 3)
		local found = {}
		for _, k in ipairs(ks) do found[k] = true end
		assert(found[1])
		assert(found[2])
		assert(found[3])
	`)
}

func TestTableKeysEmpty(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local ks = table.keys({})
		assert(#ks == 0)
	`)
}

// ===== TABLE VALUES =====

func TestTableValues(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local vs = table.values({a=1, b=2, c=3})
		assert(#vs == 3)
		-- values order is not guaranteed, so check membership
		local found = {}
		for _, v in ipairs(vs) do found[v] = true end
		assert(found[1])
		assert(found[2])
		assert(found[3])
	`)
}

func TestTableValuesEmpty(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local vs = table.values({})
		assert(#vs == 0)
	`)
}

// ===== TABLE CONTAINS =====

func TestTableContainsFound(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(table.contains({1,2,3}, 2) == true)
	`)
}

func TestTableContainsNotFound(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(table.contains({1,2,3}, 4) == false)
	`)
}

func TestTableContainsString(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(table.contains({"a","b","c"}, "b") == true)
		assert(table.contains({"a","b","c"}, "d") == false)
	`)
}

func TestTableContainsEmpty(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(table.contains({}, 1) == false)
	`)
}

func TestTableContainsNil(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		assert(table.contains({1,2,3}, nil) == false)
	`)
}

// ===== TABLE SLICE =====

func TestTableSlice(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.slice({10,20,30,40,50}, 2, 4)
		assert(#r == 3)
		assert(r[1] == 20)
		assert(r[2] == 30)
		assert(r[3] == 40)
	`)
}

func TestTableSliceNoEnd(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.slice({10,20,30,40,50}, 3)
		assert(#r == 3)
		assert(r[1] == 30)
		assert(r[2] == 40)
		assert(r[3] == 50)
	`)
}

func TestTableSliceNegativeIndices(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.slice({10,20,30,40,50}, -3, -1)
		assert(#r == 3)
		assert(r[1] == 30)
		assert(r[2] == 40)
		assert(r[3] == 50)
	`)
}

func TestTableSliceSingleElement(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.slice({10,20,30}, 2, 2)
		assert(#r == 1)
		assert(r[1] == 20)
	`)
}

func TestTableSliceOutOfBounds(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.slice({10,20,30}, 1, 100)
		assert(#r == 3)
		assert(r[1] == 10)
		assert(r[2] == 20)
		assert(r[3] == 30)
	`)
}

func TestTableSliceEmptyRange(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.slice({10,20,30}, 3, 1)
		assert(#r == 0)
	`)
}

// ===== TABLE MERGE =====

func TestTableMerge(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.merge({a=1, b=2}, {b=3, c=4})
		assert(r.a == 1)
		assert(r.b == 3) -- overridden by second table
		assert(r.c == 4)
	`)
}

func TestTableMergeMultiple(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.merge({a=1}, {b=2}, {c=3})
		assert(r.a == 1)
		assert(r.b == 2)
		assert(r.c == 3)
	`)
}

func TestTableMergeEmpty(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.merge({a=1}, {})
		assert(r.a == 1)
	`)
}

func TestTableMergeArrays(t *testing.T) {
	L := newState(t)
	doString(t, L, `
		local r = table.merge({10, 20}, {30, 40, 50})
		assert(r[1] == 30) -- overridden
		assert(r[2] == 40) -- overridden
		assert(r[3] == 50) -- new
	`)
}
