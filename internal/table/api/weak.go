// Weak table support for Lua tables (__mode metafield).
//
// Two-phase sweep approach:
// 1. Before GC: scan weak tables, create weak.Pointer for pointer-backed
//    values/keys, nil out the strong references in the table
// 2. After GC: check weak pointers — if alive, restore the value;
//    if dead (collected), leave entry as nil
//
// The actual weak.Pointer creation/resolution is done by the API layer
// via registered callbacks, because this package cannot import closure/api
// or state/api (circular dependency).
package api

import obj "github.com/akzj/go-lua/internal/object/api"

// ---------------------------------------------------------------------------
// Weak reference callbacks — set by the API layer (internal/api/api)
// ---------------------------------------------------------------------------

// WeakRefMake creates a weak reference for a pointer-backed TValue.
// Returns (weakRef as any, original TValue, true) if pointer-backed.
// Returns (nil, Nil, false) if the value is non-pointer (persists forever).
// Set by the API layer at init time.
var WeakRefMake func(v obj.TValue) (ref any, ok bool)

// WeakRefCheck checks if a weak reference is still alive.
// Returns (original TValue, true) if alive, or (Nil, false) if collected.
// Set by the API layer at init time.
var WeakRefCheck func(ref any) (val obj.TValue, alive bool)

// ---------------------------------------------------------------------------
// Two-phase sweep for weak tables
// ---------------------------------------------------------------------------

// weakEntry holds a weak reference and the original tag for one table entry.
type weakEntry struct {
	ref any     // weak.Pointer[T] as any
	tag obj.Tag // original type tag
}

// PrepareWeakSweep is phase 1 of the two-phase sweep:
// For each pointer-backed entry in the table, create a weak reference
// and nil out the strong reference. Non-pointer entries are left alone.
// Returns a restore function for phase 2.
//
// Phase 2 (the returned function): after runtime.GC(), check each weak ref.
// If alive, restore the value. If dead, leave as nil.
func (t *Table) PrepareWeakSweep() (restore func()) {
	if t.WeakMode == 0 || WeakRefMake == nil || WeakRefCheck == nil {
		return func() {}
	}

	type savedArrayEntry struct {
		idx int
		ref any
		tag obj.Tag
	}
	type savedHashEntry struct {
		nodeIdx  int
		valRef   any
		valTag   obj.Tag
		keyRef   any
		keyTag   obj.Tag
		origKey  obj.TValue // for key restoration
		origVal  obj.TValue // for value restoration
	}

	var arrEntries []savedArrayEntry
	var hashEntries []savedHashEntry

	// Phase 1a: Process array part (weak values only)
	if t.HasWeakValues() {
		for i, v := range t.Array {
			if v.Tt.IsNil() {
				continue
			}
			if ref, ok := WeakRefMake(v); ok {
				arrEntries = append(arrEntries, savedArrayEntry{
					idx: i, ref: ref, tag: v.Tt,
				})
				// Remove strong reference
				t.Array[i] = obj.Nil
			}
			// Non-pointer values stay in the table (persist forever)
		}
	}

	// Phase 1b: Process hash part
	for i := range t.Nodes {
		nd := &t.Nodes[i]
		if nd.KeyTT == obj.TagNil || nd.KeyTT == obj.TagDeadKey {
			continue
		}

		var entry savedHashEntry
		entry.nodeIdx = i
		hasWeakPart := false

		// Weak keys
		if t.HasWeakKeys() {
			keyTV := nodeKey(nd)
			if ref, ok := WeakRefMake(keyTV); ok {
				entry.keyRef = ref
				entry.keyTag = keyTV.Tt
				entry.origKey = keyTV
				entry.origVal = nd.Val
				hasWeakPart = true
			}
		}

		// Weak values
		if t.HasWeakValues() && !nd.Val.Tt.IsNil() {
			if ref, ok := WeakRefMake(nd.Val); ok {
				entry.valRef = ref
				entry.valTag = nd.Val.Tt
				entry.origVal = nd.Val
				if entry.origKey.Tt == obj.TagNil {
					entry.origKey = nodeKey(nd)
				}
				hasWeakPart = true
			}
		}

		if hasWeakPart {
			hashEntries = append(hashEntries, entry)
			// Remove strong references
			if entry.keyRef != nil {
				// Nil out the key — mark as dead so it doesn't hold a strong ref
				nd.KeyTT = obj.TagDeadKey
				nd.KeyVal = nil
				nd.Val = obj.Nil
			} else if entry.valRef != nil {
				// Only weak value — nil out the value
				nd.Val = obj.Nil
			}
		}
	}

	// Phase 2: restore function — called after runtime.GC()
	return func() {
		// Restore array entries
		for _, e := range arrEntries {
			val, alive := WeakRefCheck(e.ref)
			if alive {
				t.Array[e.idx] = val
			}
			// If dead, Array[i] is already Nil — leave it
		}

		// Restore hash entries
		for _, e := range hashEntries {
			nd := &t.Nodes[e.nodeIdx]

			keyAlive := true
			if e.keyRef != nil {
				_, keyAlive = WeakRefCheck(e.keyRef)
			}

			valAlive := true
			if e.valRef != nil {
				_, valAlive = WeakRefCheck(e.valRef)
			}

			if !keyAlive {
				// Key was collected — entire entry is dead
				nd.KeyTT = obj.TagDeadKey
				nd.KeyVal = nil
				nd.Val = obj.Nil
				continue
			}

			// Key is alive (or was non-pointer) — restore it
			if e.keyRef != nil {
				// Restore the key
				setNodeKey(nd, e.origKey)
			}

			if !valAlive {
				// Value was collected — set to nil
				nd.Val = obj.Nil
			} else if e.valRef != nil {
				// Value is alive — restore it
				val, _ := WeakRefCheck(e.valRef)
				nd.Val = val
			} else {
				// Value was non-pointer or not weak — restore original
				nd.Val = e.origVal
			}
		}
	}
}
