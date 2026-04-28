package lua

import (
	"fmt"

	"github.com/akzj/go-lua/internal/closure"
)

// ---------------------------------------------------------------------------
// Hot-reload types
// ---------------------------------------------------------------------------

// ReloadPlan represents a prepared hot-reload operation.
// Created by PrepareReload, executed by Commit.
// The plan is read-only until Commit — no state is modified during preparation.
type ReloadPlan struct {
	Module   string     // module name
	Pairs    []FuncPair // matched old↔new function pairs
	Added    []string   // new functions not in old module
	Removed  []string   // old functions not in new module
	Warnings []string   // non-fatal warnings

	// internal
	state     *State
	oldModule int  // registry ref to old module table
	newModule int  // registry ref to new module table
	committed bool // true after Commit or Abort
}

// FuncPair represents a matched pair of old and new functions.
type FuncPair struct {
	Name       string
	Compatible bool   // true if upvalue layout is compatible
	Reason     string // reason for incompatibility (if any)

	// internal
	oldClosure *closure.LClosure
	newClosure *closure.LClosure
}

// ReloadResult reports the outcome of a hot-reload operation.
type ReloadResult struct {
	Module   string
	Replaced int      // functions successfully replaced
	Skipped  int      // incompatible functions skipped
	Added    int      // new functions added to module
	Removed  int      // functions removed from old but not new (left in place)
	Warnings []string
}

// HasIncompatible returns true if any function pair is incompatible.
func (p *ReloadPlan) HasIncompatible() bool {
	for _, pair := range p.Pairs {
		if !pair.Compatible {
			return true
		}
	}
	return false
}

// IncompatibleCount returns the number of incompatible function pairs.
func (p *ReloadPlan) IncompatibleCount() int {
	count := 0
	for _, pair := range p.Pairs {
		if !pair.Compatible {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// PrepareReload — Phase 1 (read-only, no state modification)
// ---------------------------------------------------------------------------

// PrepareReload compiles new module code and creates a reload plan.
// This is Phase 1 of the two-phase commit — NO persistent state is modified.
// The old module remains in package.loaded throughout.
// Returns error if the module is not loaded or re-loading fails.
func (L *State) PrepareReload(moduleName string) (*ReloadPlan, error) {
	top := L.GetTop()
	defer L.SetTop(top)

	// 1. Get old module from package.loaded (registry["_LOADED"])
	L.GetField(RegistryIndex, "_LOADED") // push _LOADED
	loadedIdx := L.GetTop()
	tp := L.GetField(loadedIdx, moduleName) // push _LOADED[moduleName]
	if tp == TypeNil || tp == TypeNone {
		return nil, fmt.Errorf("hotreload: module %q not loaded", moduleName)
	}
	if !L.IsTable(-1) {
		return nil, fmt.Errorf("hotreload: module %q is not a table (type: %s)", moduleName, L.TypeName(L.Type(-1)))
	}
	// Save a registry ref to the old module table
	L.PushValue(-1) // dup old module
	oldRef := L.Ref(RegistryIndex)

	// 2. Clear package.loaded[moduleName] and re-require to get new version
	L.PushNil()
	L.SetField(loadedIdx, moduleName) // _LOADED[moduleName] = nil

	// Call require(moduleName) in protected mode
	L.GetGlobal("require")
	L.PushString(moduleName)
	status := L.PCall(1, 1, 0)
	if status != OK {
		// Compilation or load error — restore old module and return error
		errMsg := ""
		if s, ok := L.ToString(-1); ok {
			errMsg = s
		}
		L.Pop(1) // pop error

		// Restore old module in _LOADED
		L.RawGetI(RegistryIndex, int64(oldRef))
		L.SetField(loadedIdx, moduleName)
		L.Unref(RegistryIndex, oldRef)
		return nil, fmt.Errorf("hotreload: failed to load new %q: %s", moduleName, errMsg)
	}

	// New module is on top of stack
	if !L.IsTable(-1) {
		// New module isn't a table — restore and error
		L.Pop(1)
		L.RawGetI(RegistryIndex, int64(oldRef))
		L.SetField(loadedIdx, moduleName)
		L.Unref(RegistryIndex, oldRef)
		return nil, fmt.Errorf("hotreload: new %q is not a table", moduleName)
	}

	// Save ref to new module
	L.PushValue(-1)
	newRef := L.Ref(RegistryIndex)

	// 3. RESTORE package.loaded to old module (we haven't committed yet!)
	L.RawGetI(RegistryIndex, int64(oldRef))
	L.SetField(loadedIdx, moduleName) // _LOADED[moduleName] = old module

	// 4. Match functions between old and new modules
	plan := &ReloadPlan{
		Module:    moduleName,
		state:     L,
		oldModule: oldRef,
		newModule: newRef,
	}

	// Get old and new module table indices
	L.RawGetI(RegistryIndex, int64(oldRef)) // push old module
	oldIdx := L.GetTop()
	L.RawGetI(RegistryIndex, int64(newRef)) // push new module
	newIdx := L.GetTop()

	// Track which old functions were matched
	oldFuncs := make(map[string]*closure.LClosure)

	// Scan old module for functions
	L.PushNil()
	for L.Next(oldIdx) {
		if L.IsFunction(-1) {
			if keyStr, ok := L.ToString(-2); ok {
				cl := L.s.GetLClosure(L.GetTop())
				if cl != nil {
					oldFuncs[keyStr] = cl
				}
			}
		}
		L.Pop(1) // pop value, keep key
	}

	// Scan new module for functions, match against old
	matchedOld := make(map[string]bool)
	L.PushNil()
	for L.Next(newIdx) {
		if L.IsFunction(-1) {
			if keyStr, ok := L.ToString(-2); ok {
				newCl := L.s.GetLClosure(L.GetTop())
				if newCl != nil {
					if oldCl, exists := oldFuncs[keyStr]; exists {
						// Matched pair
						compatible, reason := checkProtoCompat(oldCl, newCl)
						plan.Pairs = append(plan.Pairs, FuncPair{
							Name:       keyStr,
							Compatible: compatible,
							Reason:     reason,
							oldClosure: oldCl,
							newClosure: newCl,
						})
						matchedOld[keyStr] = true
					} else {
						// New function not in old module
						plan.Added = append(plan.Added, keyStr)
					}
				}
			}
		}
		L.Pop(1) // pop value, keep key
	}

	// Find removed functions (in old but not in new)
	for name := range oldFuncs {
		if !matchedOld[name] {
			plan.Removed = append(plan.Removed, name)
		}
	}

	return plan, nil
}

// ---------------------------------------------------------------------------
// Commit — Phase 2 (apply changes atomically)
// ---------------------------------------------------------------------------

// Commit executes the reload plan.
// All compatible functions have their proto swapped and upvalues transferred.
// Incompatible functions are skipped. Returns the result summary.
func (p *ReloadPlan) Commit() *ReloadResult {
	if p.committed {
		return &ReloadResult{Module: p.Module, Warnings: []string{"plan already committed or aborted"}}
	}
	p.committed = true
	L := p.state

	result := &ReloadResult{
		Module:   p.Module,
		Warnings: append([]string{}, p.Warnings...),
	}

	top := L.GetTop()
	defer L.SetTop(top)

	// Push old module table for updating
	L.RawGetI(RegistryIndex, int64(p.oldModule))
	oldModIdx := L.GetTop()

	// Push new module table for reading added functions
	L.RawGetI(RegistryIndex, int64(p.newModule))
	newModIdx := L.GetTop()

	// 1. Replace compatible function pairs
	for _, pair := range p.Pairs {
		if !pair.Compatible {
			result.Skipped++
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("skipped %q: %s", pair.Name, pair.Reason))
			continue
		}

		// Transfer upvalues from old to new closure
		transferUpvalues(pair.oldClosure, pair.newClosure)

		// Swap the proto and upvals in the OLD closure so existing references
		// to the old closure see the new code. This is the key insight:
		// we mutate the old closure in-place rather than replacing the table entry,
		// so any Lua code that captured the old function value directly also gets
		// the update.
		pair.oldClosure.Proto = pair.newClosure.Proto
		pair.oldClosure.UpVals = pair.newClosure.UpVals

		result.Replaced++
	}

	// 2. Add new functions to old module table
	for _, name := range p.Added {
		L.GetField(newModIdx, name) // push new function
		L.SetField(oldModIdx, name) // old_module[name] = new_function
		result.Added++
	}

	// 3. Record removed (we don't remove them — just note them)
	result.Removed = len(p.Removed)

	// 4. Release refs
	L.Unref(RegistryIndex, p.oldModule)
	L.Unref(RegistryIndex, p.newModule)

	return result
}

// ---------------------------------------------------------------------------
// Abort — discard plan without changes
// ---------------------------------------------------------------------------

// Abort discards the reload plan without modifying any state.
func (p *ReloadPlan) Abort() {
	if p.committed {
		return
	}
	p.committed = true
	p.state.Unref(RegistryIndex, p.oldModule)
	p.state.Unref(RegistryIndex, p.newModule)
}

// ---------------------------------------------------------------------------
// Proto compatibility check
// ---------------------------------------------------------------------------

// checkProtoCompat checks if two closures have compatible upvalue layouts.
// Two closures are compatible if they have the same number of upvalues.
// Name mismatches generate warnings but don't block compatibility.
func checkProtoCompat(oldCl, newCl *closure.LClosure) (bool, string) {
	if oldCl.Proto == nil || newCl.Proto == nil {
		return false, "nil proto"
	}

	oldN := len(oldCl.Proto.Upvalues)
	newN := len(newCl.Proto.Upvalues)

	if oldN != newN {
		return false, fmt.Sprintf("upvalue count changed: %d → %d", oldN, newN)
	}

	return true, ""
}

// ---------------------------------------------------------------------------
// Upvalue transfer
// ---------------------------------------------------------------------------

// transferUpvalues transfers upvalue state from old closure to new closure.
// Uses name-based matching: for each upvalue in new, find same-named upvalue
// in old. Shares the UpVal pointer (like debug.upvaluejoin) so the live value
// is preserved across the reload.
func transferUpvalues(oldCl, newCl *closure.LClosure) {
	if oldCl.Proto == nil || newCl.Proto == nil {
		return
	}

	// Build name→index map for old closure's upvalues
	oldMap := make(map[string]int)
	for i, desc := range oldCl.Proto.Upvalues {
		if desc.Name != nil {
			oldMap[desc.Name.String()] = i
		}
	}

	// For each upvalue in new closure, try to find matching old upvalue
	for i, desc := range newCl.Proto.Upvalues {
		if desc.Name == nil {
			continue
		}
		name := desc.Name.String()
		if oldIdx, ok := oldMap[name]; ok {
			if oldCl.UpVals[oldIdx] != nil {
				// Share the UpVal pointer — same semantics as debug.upvaluejoin
				newCl.UpVals[i] = oldCl.UpVals[oldIdx]
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Convenience API
// ---------------------------------------------------------------------------

// ReloadModule is a convenience function that prepares and commits in one step.
// If preparation fails, returns error (no state modified).
// Incompatible functions are skipped (partial update).
func (L *State) ReloadModule(name string) (*ReloadResult, error) {
	plan, err := L.PrepareReload(name)
	if err != nil {
		return nil, err
	}
	return plan.Commit(), nil
}
