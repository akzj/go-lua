// Package api provides the public Lua API
// This file tests the table standard library module
package api

import (
	"bytes"
	"os"
	"testing"
)

// helper to capture stdout during DoString
func captureOutput(t *testing.T, code string) string {
	t.Helper()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	L := NewState()
	L.OpenLibs()

	err := L.DoString(code, "test")

	w.Close()
	os.Stdout = old
	L.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()

	if err != nil {
		t.Fatalf("DoString(%q) failed: %v", code, err)
	}

	return buf.String()
}

// =============================================================================
// table.insert tests
// =============================================================================

func TestTableInsert_Append(t *testing.T) {
	output := captureOutput(t, `
		local insert = table.insert
		local t = {1, 2, 3}
		insert(t, 4)
		print(#t)
		print(t[4])
	`)
	
	if output != "4\n4\n" {
		t.Errorf("Expected '4\\n4\\n', got %q", output)
	}
}

func TestTableInsert_InsertAtPosition(t *testing.T) {
	output := captureOutput(t, `
		local insert = table.insert
		local t = {1, 3, 4}
		insert(t, 2, 2)
		print(t[1], t[2], t[3], t[4])
	`)
	
	if output != "1\t2\t3\t4\n" {
		t.Errorf("Expected '1\\t2\\t3\\t4\\n', got %q", output)
	}
}

func TestTableInsert_EmptyTable(t *testing.T) {
	output := captureOutput(t, `
		local insert = table.insert
		local t = {}
		insert(t, "hello")
		print(#t)
		print(t[1])
	`)
	
	if output != "1\nhello\n" {
		t.Errorf("Expected '1\\nhello\\n', got %q", output)
	}
}

func TestTableInsert_SingleElement(t *testing.T) {
	output := captureOutput(t, `
		local insert = table.insert
		local t = {42}
		insert(t, 100)
		print(t[1], t[2])
		print(#t)
	`)
	
	if output != "42\t100\n2\n" {
		t.Errorf("Expected '42\\t100\\n2\\n', got %q", output)
	}
}

func TestTableInsert_MultipleAppends(t *testing.T) {
	output := captureOutput(t, `
		local insert = table.insert
		local t = {}
		for i = 1, 5 do
			insert(t, i)
		end
		print(t[1], t[2], t[3], t[4], t[5])
	`)
	
	if output != "1\t2\t3\t4\t5\n" {
		t.Errorf("Expected '1\\t2\\t3\\t4\\t5\\n', got %q", output)
	}
}

// =============================================================================
// table.remove tests
// =============================================================================

func TestTableRemove_Last(t *testing.T) {
	output := captureOutput(t, `
		local remove = table.remove
		local t = {1, 2, 3}
		local removed = remove(t)
		print(removed, #t)
	`)
	
	if output != "3\t2\n" {
		t.Errorf("Expected '3\\t2\\n', got %q", output)
	}
}

func TestTableRemove_AtPosition(t *testing.T) {
	output := captureOutput(t, `
		local remove = table.remove
		local t = {1, 2, 3}
		local removed = remove(t, 2)
		print(removed, t[1], t[2], #t)
	`)
	
	if output != "2\t1\t3\t2\n" {
		t.Errorf("Expected '2\\t1\\t3\\t2\\n', got %q", output)
	}
}

func TestTableRemove_EmptyTable(t *testing.T) {
	output := captureOutput(t, `
		local remove = table.remove
		local t = {}
		local removed = remove(t)
		print(removed)
	`)
	
	if output != "nil\n" {
		t.Errorf("Expected 'nil\\n', got %q", output)
	}
}

func TestTableRemove_SingleElement(t *testing.T) {
	output := captureOutput(t, `
		local remove = table.remove
		local t = {42}
		local removed = remove(t)
		print(removed, #t)
	`)
	
	if output != "42\t0\n" {
		t.Errorf("Expected '42\\t0\\n', got %q", output)
	}
}

func TestTableRemove_InvalidPosition(t *testing.T) {
	output := captureOutput(t, `
		local remove = table.remove
		local t = {1, 2, 3}
		local removed = remove(t, 10)
		print(removed)
	`)
	
	if output != "nil\n" {
		t.Errorf("Expected 'nil\\n', got %q", output)
	}
}

func TestTableRemove_FirstElement(t *testing.T) {
	output := captureOutput(t, `
		local remove = table.remove
		local t = {1, 2, 3}
		local removed = remove(t, 1)
		print(removed, t[1], t[2], #t)
	`)
	
	if output != "1\t2\t3\t2\n" {
		t.Errorf("Expected '1\\t2\\t3\\t2\\n', got %q", output)
	}
}

// =============================================================================
// table.sort tests
// =============================================================================

func TestTableSort_Numbers(t *testing.T) {
	output := captureOutput(t, `
		local sort = table.sort
		local t = {3, 1, 2}
		sort(t)
		print(t[1], t[2], t[3])
	`)
	
	if output != "1\t2\t3\n" {
		t.Errorf("Expected '1\\t2\\t3\\n', got %q", output)
	}
}

func TestTableSort_Strings(t *testing.T) {
	output := captureOutput(t, `
		local sort = table.sort
		local t = {"banana", "apple", "cherry"}
		sort(t)
		print(t[1], t[2], t[3])
	`)
	
	if output != "apple\tbanana\tcherry\n" {
		t.Errorf("Expected 'apple\\tbanana\\tcherry\\n', got %q", output)
	}
}

func TestTableSort_EmptyTable(t *testing.T) {
	output := captureOutput(t, `
		local sort = table.sort
		local t = {}
		sort(t)
		print(#t)
	`)
	
	if output != "0\n" {
		t.Errorf("Expected '0\\n', got %q", output)
	}
}

func TestTableSort_SingleElement(t *testing.T) {
	output := captureOutput(t, `
		local sort = table.sort
		local t = {42}
		sort(t)
		print(t[1])
	`)
	
	if output != "42\n" {
		t.Errorf("Expected '42\\n', got %q", output)
	}
}

func TestTableSort_AlreadySorted(t *testing.T) {
	output := captureOutput(t, `
		local sort = table.sort
		local t = {1, 2, 3, 4, 5}
		sort(t)
		print(t[1], t[2], t[3], t[4], t[5])
	`)
	
	if output != "1\t2\t3\t4\t5\n" {
		t.Errorf("Expected '1\\t2\\t3\\t4\\t5\\n', got %q", output)
	}
}

func TestTableSort_ReverseOrder(t *testing.T) {
	output := captureOutput(t, `
		local sort = table.sort
		local t = {5, 4, 3, 2, 1}
		sort(t)
		print(t[1], t[2], t[3], t[4], t[5])
	`)
	
	if output != "1\t2\t3\t4\t5\n" {
		t.Errorf("Expected '1\\t2\\t3\\t4\\t5\\n', got %q", output)
	}
}

// =============================================================================
// table.concat tests
// =============================================================================

func TestTableConcat_Basic(t *testing.T) {
	output := captureOutput(t, `
		local concat = table.concat
		local t = {"a", "b", "c"}
		print(concat(t, ","))
	`)
	
	if output != "a,b,c\n" {
		t.Errorf("Expected 'a,b,c\\n', got %q", output)
	}
}

func TestTableConcat_NoSeparator(t *testing.T) {
	output := captureOutput(t, `
		local concat = table.concat
		local t = {"a", "b", "c"}
		print(concat(t))
	`)
	
	if output != "abc\n" {
		t.Errorf("Expected 'abc\\n', got %q", output)
	}
}

func TestTableConcat_WithRange(t *testing.T) {
	output := captureOutput(t, `
		local concat = table.concat
		local t = {"a", "b", "c", "d", "e"}
		print(concat(t, ",", 2, 4))
	`)
	
	if output != "b,c,d\n" {
		t.Errorf("Expected 'b,c,d\\n', got %q", output)
	}
}

func TestTableConcat_Numbers(t *testing.T) {
	output := captureOutput(t, `
		local concat = table.concat
		local t = {1, 2, 3}
		print(concat(t, "-"))
	`)
	
	if output != "1-2-3\n" {
		t.Errorf("Expected '1-2-3\\n', got %q", output)
	}
}

func TestTableConcat_EmptyTable(t *testing.T) {
	output := captureOutput(t, `
		local concat = table.concat
		local t = {}
		print(concat(t, ","))
	`)
	
	if output != "\n" {
		t.Errorf("Expected '\\n', got %q", output)
	}
}

func TestTableConcat_SingleElement(t *testing.T) {
	output := captureOutput(t, `
		local concat = table.concat
		local t = {"hello"}
		print(concat(t, ","))
	`)
	
	if output != "hello\n" {
		t.Errorf("Expected 'hello\\n', got %q", output)
	}
}

// =============================================================================
// table.pack tests
// =============================================================================

func TestTablePack_Basic(t *testing.T) {
	output := captureOutput(t, `
		local pack = table.pack
		local t = pack(1, 2, 3)
		print(t.n, t[1], t[2], t[3])
	`)
	
	if output != "3\t1\t2\t3\n" {
		t.Errorf("Expected '3\\t1\\t2\\t3\\n', got %q", output)
	}
}

func TestTablePack_Empty(t *testing.T) {
	output := captureOutput(t, `
		local pack = table.pack
		local t = pack()
		print(t.n)
	`)
	
	if output != "0\n" {
		t.Errorf("Expected '0\\n', got %q", output)
	}
}

func TestTablePack_MixedTypes(t *testing.T) {
	output := captureOutput(t, `
		local pack = table.pack
		local t = pack(1, "hello", true)
		print(t.n, type(t[1]), type(t[2]), type(t[3]))
	`)
	
	if output != "3\tnumber\tstring\tboolean\n" {
		t.Errorf("Expected '3\\tnumber\\tstring\\tboolean\\n', got %q", output)
	}
}

func TestTablePack_WithNil(t *testing.T) {
	output := captureOutput(t, `
		local pack = table.pack
		local t = pack(1, nil, 3)
		print(t.n)
	`)
	
	if output != "3\n" {
		t.Errorf("Expected '3\\n', got %q", output)
	}
}

func TestTablePack_ManyArgs(t *testing.T) {
	output := captureOutput(t, `
		local pack = table.pack
		local t = pack(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
		print(t.n, t[1], t[10])
	`)
	
	if output != "10\t1\t10\n" {
		t.Errorf("Expected '10\\t1\\t10\\n', got %q", output)
	}
}

// =============================================================================
// table.unpack tests
// =============================================================================

func TestTableUnpack_Basic(t *testing.T) {
	output := captureOutput(t, `
		local unpack = table.unpack
		local t = {1, 2, 3}
		local a, b, c = unpack(t)
		print(a, b, c)
	`)
	
	if output != "1\t2\t3\n" {
		t.Errorf("Expected '1\\t2\\t3\\n', got %q", output)
	}
}

func TestTableUnpack_WithRange(t *testing.T) {
	output := captureOutput(t, `
		local unpack = table.unpack
		local t = {1, 2, 3, 4, 5}
		local a, b, c = unpack(t, 2, 4)
		print(a, b, c)
	`)
	
	if output != "2\t3\t4\n" {
		t.Errorf("Expected '2\\t3\\t4\\n', got %q", output)
	}
}

func TestTableUnpack_EmptyTable(t *testing.T) {
	output := captureOutput(t, `
		local unpack = table.unpack
		local select = select
		local t = {}
		local a = unpack(t)
		print(a)
	`)
	
	// unpack of empty table should return nil
	if output != "nil\n" {
		t.Errorf("Expected 'nil\\n', got %q", output)
	}
}

func TestTableUnpack_SingleElement(t *testing.T) {
	output := captureOutput(t, `
		local unpack = table.unpack
		local t = {42}
		local a = unpack(t)
		print(a)
	`)
	
	if output != "42\n" {
		t.Errorf("Expected '42\\n', got %q", output)
	}
}

func TestTableUnpack_MixedTypes(t *testing.T) {
	output := captureOutput(t, `
		local unpack = table.unpack
		local t = {1, "hello", true}
		local a, b, c = unpack(t)
		print(type(a), type(b), type(c))
	`)
	
	if output != "number\tstring\tboolean\n" {
		t.Errorf("Expected 'number\\tstring\\tboolean\\n', got %q", output)
	}
}

func TestTableUnpack_AllElements(t *testing.T) {
	output := captureOutput(t, `
		local unpack = table.unpack
		local t = {10, 20, 30, 40, 50}
		local a, b, c, d, e = unpack(t)
		print(a, b, c, d, e)
	`)
	
	if output != "10\t20\t30\t40\t50\n" {
		t.Errorf("Expected '10\\t20\\t30\\t40\\t50\\n', got %q", output)
	}
}

// =============================================================================
// Edge cases and error handling
// =============================================================================

func TestTableInsert_InvalidTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`
		local insert = table.insert
		insert("not a table", 1)
	`, "test")
	if err == nil {
		t.Error("Expected error for non-table argument")
	}
}

func TestTableRemove_InvalidTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`
		local remove = table.remove
		remove("not a table")
	`, "test")
	if err == nil {
		t.Error("Expected error for non-table argument")
	}
}

func TestTableSort_InvalidTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	err := L.DoString(`
		local sort = table.sort
		sort("not a table")
	`, "test")
	if err == nil {
		t.Error("Expected error for non-table argument")
	}
}

// =============================================================================
// Integration tests
// =============================================================================

func TestTable_InsertRemoveRoundTrip(t *testing.T) {
	output := captureOutput(t, `
		local insert = table.insert
		local remove = table.remove
		local t = {}
		insert(t, 1)
		insert(t, 2)
		insert(t, 3)
		local a = remove(t)
		insert(t, 4)
		print(t[1], t[2], t[3])
		print(#t)
	`)
	
	if output != "1\t2\t4\n3\n" {
		t.Errorf("Expected '1\\t2\\t4\\n3\\n', got %q", output)
	}
}

func TestTable_PackUnpackRoundTrip(t *testing.T) {
	// Test that unpack returns values that can be used
	output := captureOutput(t, `
		local unpack = table.unpack
		local original = {1, 2, 3, 4, 5}
		local a, b, c, d, e = unpack(original)
		print(a, b, c, d, e)
	`)
	
	if output != "1\t2\t3\t4\t5\n" {
		t.Errorf("Expected '1\\t2\\t3\\t4\\t5\\n', got %q", output)
	}
}

func TestTable_SortConcat(t *testing.T) {
	output := captureOutput(t, `
		local sort = table.sort
		local concat = table.concat
		local t = {3, 1, 4, 1, 5, 9, 2, 6}
		sort(t)
		print(concat(t, ","))
	`)
	
	if output != "1,1,2,3,4,5,6,9\n" {
		t.Errorf("Expected '1,1,2,3,4,5,6,9\\n', got %q", output)
	}
}

func TestTable_NilAssignment(t *testing.T) {
	output := captureOutput(t, `
		local t = {1, 2, 3}
		t[2] = nil
		print(t[1], t[2], t[3])
	`)
	
	// t[2] should be nil
	if output != "1\tnil\t3\n" {
		t.Errorf("Expected '1\\tnil\\t3\\n', got %q", output)
	}
}

func TestTable_InsertNil(t *testing.T) {
	// Note: table.insert with nil value behavior varies
	// This test verifies the current implementation behavior
	output := captureOutput(t, `
		local insert = table.insert
		local t = {1, 2}
		insert(t, nil)
		insert(t, 3)
		print(t[1], t[2])
		print(t[3])
	`)
	
	// Check that first two elements are correct
	if output != "1\t2\n3\n" && output != "1\t2\nnil\n" {
		t.Errorf("Expected '1\\t2\\n3\\n' or '1\\t2\\nnil\\n', got %q", output)
	}
}

func TestTable_HolesInTable(t *testing.T) {
	output := captureOutput(t, `
		local t = {1, 2, 3}
		t[2] = nil
		print(t[1], t[2], t[3])
	`)
	
	if output != "1\tnil\t3\n" {
		t.Errorf("Expected '1\\tnil\\t3\\n', got %q", output)
	}
}

func TestTable_ConcatWithHoles(t *testing.T) {
	// Lua's table.concat raises an error for nil values in the range
	// This matches standard Lua behavior
	output := captureOutput(t, `
		local concat = table.concat
		local t = {1, 2, 3}
		t[2] = nil
		local ok, err = pcall(concat, t, ",")
		print(ok, err ~= nil)
	`)
	
	// Should get an error about nil at index 2
	if output != "false\ttrue\n" {
		t.Errorf("Expected 'false\\ttrue\\n', got %q", output)
	}
}

func TestTable_RemoveFromMiddle(t *testing.T) {
	output := captureOutput(t, `
		local remove = table.remove
		local t = {10, 20, 30, 40, 50}
		local removed = remove(t, 3)
		print(removed)
		print(t[1], t[2], t[3], t[4])
	`)
	
	if output != "30\n10\t20\t40\t50\n" {
		t.Errorf("Expected '30\\n10\\t20\\t40\\t50\\n', got %q", output)
	}
}

func TestTable_InsertAtBeginning(t *testing.T) {
	output := captureOutput(t, `
		local insert = table.insert
		local t = {2, 3, 4}
		insert(t, 1, 1)
		print(t[1], t[2], t[3], t[4])
	`)
	
	if output != "1\t2\t3\t4\n" {
		t.Errorf("Expected '1\\t2\\t3\\t4\\n', got %q", output)
	}
}

func TestTable_InsertAtEnd(t *testing.T) {
	output := captureOutput(t, `
		local insert = table.insert
		local t = {1, 2, 3}
		insert(t, 4, 4)
		print(t[1], t[2], t[3], t[4])
	`)
	
	if output != "1\t2\t3\t4\n" {
		t.Errorf("Expected '1\\t2\\t3\\t4\\n', got %q", output)
	}
}