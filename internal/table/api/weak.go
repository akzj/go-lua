// Weak table support for Lua tables (__mode metafield).
//
// Two-phase sweep approach:
// 1. Before GC: scan weak tables, create weak.Pointer for pointer-backed
//    values/keys, nil out the strong references in the table
// 2. After GC: check weak pointers — if alive, restore the value;
//    if dead (collected), leave entry as nil
//
// CRITICAL: No strong references to GC-collectable Lua objects may be held
// during GC. The closures returned by PrepareWeakSweep capture only:
// - weak refs (which hold weak.Pointer, not strong pointers)
// - non-pointer values (int64, float64, bool, strings) which are safe
//   because Go GC doesn't collect them
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
// Returns (weakRef as any, true) if pointer-backed.
// Returns (nil, false) if the value is non-pointer (persists forever).
// The returned weakRef must NOT hold a strong reference to the value.
// Set by the API layer at init time.
var WeakRefMake func(v obj.TValue) (ref any, ok bool)

// WeakRefCheck checks if a weak reference is still alive.
// If alive, reconstructs the TValue from the weak pointer's .Value().
// Returns (reconstructed TValue, true) if alive, or (Nil, false) if collected.
// Set by the API layer at init time.
var WeakRefCheck func(ref any) (val obj.TValue, alive bool)

// ---------------------------------------------------------------------------
// Two-phase sweep for weak tables
// ---------------------------------------------------------------------------

// PrepareWeakSweep is phase 1 of the two-phase sweep:
// For each pointer-backed entry in the table, create a weak reference
// and nil out the strong reference. Non-pointer entries are left alone.
// Returns a restore function for phase 2.
//
// Phase 2 (the returned function): after runtime.GC(), check each weak ref.
// If alive, restore the value (reconstructed from weak pointer). If dead, leave as nil.
//
// CRITICAL: The returned closure must NOT capture any strong references to
// GC-collectable Lua objects. Only weak refs and non-pointer values are safe.
func (t *Table) PrepareWeakSweep() (restore func()) {
	if t.WeakMode == 0 || WeakRefMake == nil || WeakRefCheck == nil {
		return func() {}
	}

	// Per-entry saved state. Only holds weak refs and non-pointer values.
	type arraySaved struct {
		idx     int
		weakRef any // from WeakRefMake — only holds weak.Pointer
	}

	type hashSaved struct {
		nodeIdx int
		keyRef  any // weak ref for key (nil if non-pointer or not weak)
		valRef  any // weak ref for value (nil if non-pointer or not weak)
		// For entries with weak key but non-pointer value: save the value.
		// Non-pointer TValues (int64, float64, bool, strings) are safe to hold
		// because Go GC doesn't collect them — they're value types.
		nonPtrVal    obj.TValue // only set when key is weak and value is non-pointer
		hasNonPtrVal bool
	}

	var arrSaved []arraySaved
	var hashSaved_ []hashSaved

	// Phase 1a: Process array part (weak values only)
	if t.HasWeakValues() {
		for i, v := range t.Array {
			if v.Tt.IsNil() {
				continue
			}
			if ref, ok := WeakRefMake(v); ok {
				arrSaved = append(arrSaved, arraySaved{idx: i, weakRef: ref})
				// Remove strong reference from table
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

		var entry hashSaved
		entry.nodeIdx = i
		hasWeakPart := false

		// Weak keys: create weak ref for the key
		if t.HasWeakKeys() {
			keyTV := nodeKey(nd)
			if ref, ok := WeakRefMake(keyTV); ok {
				entry.keyRef = ref
				hasWeakPart = true
			}
		}

		// Weak values: create weak ref for the value
		if t.HasWeakValues() && !nd.Val.Tt.IsNil() {
			if ref, ok := WeakRefMake(nd.Val); ok {
				entry.valRef = ref
				hasWeakPart = true
			}
		}

		if !hasWeakPart {
			continue
		}

		// Remove strong references from table
		if entry.keyRef != nil {
			// Key is pointer-backed and weak — nil it out
			// Save non-pointer value before nilling (safe to hold)
			if entry.valRef == nil && !nd.Val.Tt.IsNil() {
				entry.nonPtrVal = nd.Val
				entry.hasNonPtrVal = true
			}
			nd.KeyTT = obj.TagDeadKey
			nd.KeyVal = nil
			nd.Val = obj.Nil
		} else if entry.valRef != nil {
			// Only value is weak — nil out just the value
			nd.Val = obj.Nil
		}

		hashSaved_ = append(hashSaved_, entry)
	}

	// Phase 2: restore function — called after runtime.GC()
	// This closure captures ONLY weak refs and non-pointer values.
	return func() {
		// Restore array entries
		for _, e := range arrSaved {
			val, alive := WeakRefCheck(e.weakRef)
			if alive {
				t.Array[e.idx] = val
			}
			// If dead, Array[idx] is already Nil — leave it
		}

		// Restore hash entries
		for _, e := range hashSaved_ {
			nd := &t.Nodes[e.nodeIdx]

			if e.keyRef != nil {
				// Key was weak — check if it survived
				keyVal, keyAlive := WeakRefCheck(e.keyRef)
				if !keyAlive {
					// Key was collected — entire entry is dead
					nd.KeyTT = obj.TagDeadKey
					nd.KeyVal = nil
					nd.Val = obj.Nil
					continue
				}
				// Key is alive — restore it
				setNodeKey(nd, keyVal)

				// Restore value
				if e.valRef != nil {
					// Value was also weak
					val, valAlive := WeakRefCheck(e.valRef)
					if valAlive {
						nd.Val = val
					} else {
						nd.Val = obj.Nil
					}
				} else if e.hasNonPtrVal {
					// Value was non-pointer — restore from saved copy
					nd.Val = e.nonPtrVal
				}
				// If neither, value was already Nil before sweep
			} else if e.valRef != nil {
				// Only value was weak — key is still in node (wasn't nil'd)
				val, valAlive := WeakRefCheck(e.valRef)
				if valAlive {
					nd.Val = val
				}
				// If dead, nd.Val is already Nil
			}
		}
	}
}
