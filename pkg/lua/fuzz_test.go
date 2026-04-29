package lua

import (
	"testing"
)

// FuzzDoString fuzzes the Lua parser + VM with arbitrary code strings.
// Should never panic — errors are expected, panics are bugs.
func FuzzDoString(f *testing.F) {
	// Seed corpus with valid Lua
	f.Add("return 42")
	f.Add("local x = 1 + 2")
	f.Add("for i=1,10 do end")
	f.Add("local t = {1,2,3}; return #t")
	f.Add("function f(x) return x+1 end; return f(41)")
	f.Add("local co = coroutine.create(function() coroutine.yield(1) end); coroutine.resume(co)")
	f.Add("string.find('hello world', 'wor')")
	f.Add("local t = setmetatable({}, {__index=function(t,k) return k end}); return t.abc")
	f.Add("error('test')")
	f.Add("pcall(error, 'test')")
	f.Add("")
	f.Add("return ...")
	f.Add("do end")
	f.Add("--[[ comment ]]")

	f.Fuzz(func(t *testing.T, code string) {
		L := NewState()
		defer L.Close()
		// Should never panic. Errors are fine.
		_ = L.DoString(code)
	})
}

// FuzzRefUnref fuzzes the Ref/Unref free list with random operation sequences.
func FuzzRefUnref(f *testing.F) {
	// Seed: sequence of bytes where even=Ref, odd=Unref(random existing ref)
	f.Add([]byte{0, 0, 0, 1, 1, 0, 0, 1})
	f.Add([]byte{0, 1, 0, 1, 0, 1})
	f.Add([]byte{0, 0, 1, 1, 1, 1}) // more unrefs than refs
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0}) // all refs

	f.Fuzz(func(t *testing.T, ops []byte) {
		L := NewState()
		defer L.Close()

		var refs []int
		seen := make(map[int]bool)

		for _, op := range ops {
			if op%2 == 0 {
				// Ref
				L.PushInteger(42)
				ref := L.Ref(RegistryIndex)
				if ref > 0 {
					if seen[ref] {
						t.Fatalf("Ref returned duplicate slot %d", ref)
					}
					seen[ref] = true
					refs = append(refs, ref)
				}
			} else {
				// Unref (possibly double-unref, possibly invalid ref)
				if len(refs) > 0 {
					idx := int(op) % len(refs)
					ref := refs[idx]
					L.Unref(RegistryIndex, ref)
					delete(seen, ref)
					// Remove from refs slice
					refs[idx] = refs[len(refs)-1]
					refs = refs[:len(refs)-1]
				}
			}
		}

		// All remaining refs should be retrievable
		for _, ref := range refs {
			L.RawGetI(RegistryIndex, int64(ref))
			if L.IsNil(-1) {
				t.Fatalf("ref %d was lost", ref)
			}
			L.Pop(1)
		}
	})
}

// FuzzStackOps fuzzes stack manipulation operations.
func FuzzStackOps(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 10, 11, 12})

	f.Fuzz(func(t *testing.T, ops []byte) {
		L := NewState()
		defer L.Close()

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic during stack ops: %v", r)
			}
		}()

		for _, op := range ops {
			top := L.GetTop()
			switch op % 8 {
			case 0: // Push
				L.PushInteger(int64(op))
			case 1: // Pop (if stack non-empty)
				if top > 0 {
					L.Pop(1)
				}
			case 2: // PushValue (duplicate top)
				if top > 0 {
					L.PushValue(-1)
				}
			case 3: // SetTop (shrink)
				if top > 0 {
					L.SetTop(top - 1)
				}
			case 4: // PushNil
				L.PushNil()
			case 5: // PushString
				L.PushString("test")
			case 6: // PushBoolean
				L.PushBoolean(op > 128)
			case 7: // GetTop (read-only, always safe)
				_ = L.GetTop()
			}
		}
	})
}
