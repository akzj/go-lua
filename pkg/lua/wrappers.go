package lua

// luaArg reads a typed argument from the Lua stack at position idx.
// It uses a type switch on the zero value to dispatch to the correct
// Lua-to-Go conversion without reflection.
func luaArg[T any](L *State, idx int) T {
	var zero T
	switch any(zero).(type) {
	case string:
		s, _ := L.ToString(idx)
		return any(s).(T)
	case int:
		n, _ := L.ToInteger(idx)
		return any(int(n)).(T)
	case int64:
		n, _ := L.ToInteger(idx)
		return any(n).(T)
	case float64:
		n, _ := L.ToNumber(idx)
		return any(n).(T)
	case float32:
		n, _ := L.ToNumber(idx)
		return any(float32(n)).(T)
	case bool:
		return any(L.ToBoolean(idx)).(T)
	case map[string]any:
		m, _ := L.ToMap(idx)
		return any(m).(T)
	default:
		val := L.ToAny(idx)
		if v, ok := val.(T); ok {
			return v
		}
		return zero
	}
}

// ---------------------------------------------------------------------------
// 0 args
// ---------------------------------------------------------------------------

// Wrap0 wraps func() as a Lua function (no args, no return).
func Wrap0(L *State, fn func()) {
	L.PushFunction(func(L *State) int {
		fn()
		return 0
	})
}

// Wrap0R wraps func() R as a Lua function (no args, 1 return).
func Wrap0R[R any](L *State, fn func() R) {
	L.PushFunction(func(L *State) int {
		L.PushAny(any(fn()))
		return 1
	})
}

// Wrap0E wraps func() error as a Lua function (no args, error return).
func Wrap0E(L *State, fn func() error) {
	L.PushFunction(func(L *State) int {
		if err := fn(); err != nil {
			return L.Errorf("%s", err.Error())
		}
		return 0
	})
}

// ---------------------------------------------------------------------------
// 1 arg
// ---------------------------------------------------------------------------

// Wrap1 wraps func(A) as a Lua function.
func Wrap1[A any](L *State, fn func(A)) {
	L.PushFunction(func(L *State) int {
		fn(luaArg[A](L, 1))
		return 0
	})
}

// Wrap1R wraps func(A) R as a Lua function.
func Wrap1R[A, R any](L *State, fn func(A) R) {
	L.PushFunction(func(L *State) int {
		L.PushAny(any(fn(luaArg[A](L, 1))))
		return 1
	})
}

// Wrap1E wraps func(A) (R, error) as a Lua function.
func Wrap1E[A, R any](L *State, fn func(A) (R, error)) {
	L.PushFunction(func(L *State) int {
		r, err := fn(luaArg[A](L, 1))
		if err != nil {
			return L.Errorf("%s", err.Error())
		}
		L.PushAny(any(r))
		return 1
	})
}

// ---------------------------------------------------------------------------
// 2 args
// ---------------------------------------------------------------------------

// Wrap2 wraps func(A, B) as a Lua function.
func Wrap2[A, B any](L *State, fn func(A, B)) {
	L.PushFunction(func(L *State) int {
		fn(luaArg[A](L, 1), luaArg[B](L, 2))
		return 0
	})
}

// Wrap2R wraps func(A, B) R as a Lua function.
func Wrap2R[A, B, R any](L *State, fn func(A, B) R) {
	L.PushFunction(func(L *State) int {
		L.PushAny(any(fn(luaArg[A](L, 1), luaArg[B](L, 2))))
		return 1
	})
}

// Wrap2E wraps func(A, B) (R, error) as a Lua function.
func Wrap2E[A, B, R any](L *State, fn func(A, B) (R, error)) {
	L.PushFunction(func(L *State) int {
		r, err := fn(luaArg[A](L, 1), luaArg[B](L, 2))
		if err != nil {
			return L.Errorf("%s", err.Error())
		}
		L.PushAny(any(r))
		return 1
	})
}

// ---------------------------------------------------------------------------
// 3 args
// ---------------------------------------------------------------------------

// Wrap3 wraps func(A, B, C) as a Lua function.
func Wrap3[A, B, C any](L *State, fn func(A, B, C)) {
	L.PushFunction(func(L *State) int {
		fn(luaArg[A](L, 1), luaArg[B](L, 2), luaArg[C](L, 3))
		return 0
	})
}

// Wrap3R wraps func(A, B, C) R as a Lua function.
func Wrap3R[A, B, C, R any](L *State, fn func(A, B, C) R) {
	L.PushFunction(func(L *State) int {
		L.PushAny(any(fn(luaArg[A](L, 1), luaArg[B](L, 2), luaArg[C](L, 3))))
		return 1
	})
}
