package lua

func init() {
	RegisterGlobal("hotreload", OpenHotReload)
}

// OpenHotReload opens the "hotreload" module.
//
// Lua API:
//
//	local hr = require("hotreload")
//	local result, err = hr.reload("modulename")
//	local plan, err = hr.prepare("modulename")
//	if plan then plan:commit() end
func OpenHotReload(L *State) {
	L.NewLib(map[string]Function{
		"reload":  hotreloadReload,
		"prepare": hotreloadPrepare,
	})
}

// hr.reload(name) — reload module, return result table or nil+error
func hotreloadReload(L *State) int {
	name := L.CheckString(1)
	result, err := L.ReloadModule(name)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}
	pushReloadResult(L, result)
	return 1
}

// hr.prepare(name) — prepare reload, return plan userdata or nil+error
func hotreloadPrepare(L *State) int {
	name := L.CheckString(1)
	plan, err := L.PrepareReload(name)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	// Store plan as userdata with metatable for commit/abort methods
	L.PushUserdata(plan)
	planIdx := L.GetTop()

	// Create or get the "HotReloadPlan" metatable
	if L.NewMetatable("HotReloadPlan") {
		// First time: populate metatable
		L.PushFunction(hotreloadPlanCommit)
		L.SetField(-2, "commit")
		L.PushFunction(hotreloadPlanAbort)
		L.SetField(-2, "abort")
		L.PushFunction(hotreloadPlanInfo)
		L.SetField(-2, "info")

		// __index = metatable itself (so plan:commit() works)
		L.PushValue(-1)
		L.SetField(-2, "__index")
	}
	L.SetMetatable(planIdx)

	return 1
}

// plan:commit() — commit the reload plan, return result table
func hotreloadPlanCommit(L *State) int {
	ud := L.CheckUserdata(1)
	plan, ok := ud.(*ReloadPlan)
	if !ok {
		L.ArgError(1, "expected HotReloadPlan")
		return 0
	}
	result := plan.Commit()
	pushReloadResult(L, result)
	return 1
}

// plan:abort() — abort the reload plan
func hotreloadPlanAbort(L *State) int {
	ud := L.CheckUserdata(1)
	plan, ok := ud.(*ReloadPlan)
	if !ok {
		L.ArgError(1, "expected HotReloadPlan")
		return 0
	}
	plan.Abort()
	return 0
}

// plan:info() — return info table about the plan
func hotreloadPlanInfo(L *State) int {
	ud := L.CheckUserdata(1)
	plan, ok := ud.(*ReloadPlan)
	if !ok {
		L.ArgError(1, "expected HotReloadPlan")
		return 0
	}

	L.NewTable()
	L.PushString(plan.Module)
	L.SetField(-2, "module")
	L.PushInteger(int64(len(plan.Pairs)))
	L.SetField(-2, "matched")
	L.PushInteger(int64(len(plan.Added)))
	L.SetField(-2, "added")
	L.PushInteger(int64(len(plan.Removed)))
	L.SetField(-2, "removed")
	L.PushInteger(int64(plan.IncompatibleCount()))
	L.SetField(-2, "incompatible")
	return 1
}

// pushReloadResult pushes a ReloadResult as a Lua table.
func pushReloadResult(L *State, r *ReloadResult) {
	L.NewTable()
	L.PushString(r.Module)
	L.SetField(-2, "module")
	L.PushInteger(int64(r.Replaced))
	L.SetField(-2, "replaced")
	L.PushInteger(int64(r.Skipped))
	L.SetField(-2, "skipped")
	L.PushInteger(int64(r.Added))
	L.SetField(-2, "added")
	L.PushInteger(int64(r.Removed))
	L.SetField(-2, "removed")

	// Warnings array
	if len(r.Warnings) > 0 {
		L.CreateTable(len(r.Warnings), 0)
		for i, w := range r.Warnings {
			L.PushString(w)
			L.RawSetI(-2, int64(i+1))
		}
		L.SetField(-2, "warnings")
	}
}
