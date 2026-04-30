package lua

import "io"

// Closeable is any Go resource that can be closed.
// Matches io.Closer.
type Closeable = io.Closer

// PushResource pushes a Go resource as userdata with a __gc metamethod.
// When Lua's GC collects this userdata, the resource's Close() is called.
// This prevents Go resource leaks when Lua code forgets to close explicitly.
//
// The metatable is named "go.resource" and provides:
//   - __gc: calls resource.Close()
//   - close: explicit close (idempotent)
//
// Example:
//
//	conn := db.Open(...)
//	L.PushResource(conn)
//	L.SetGlobal("conn")
//	// conn.Close() will be called when GC collects it or Lua calls conn:close()
func (L *State) PushResource(resource Closeable) {
	L.PushUserdata(&luaResource{resource: resource})
	setResourceMetatable(L)
}

// PushCloseableResource pushes a Go resource with both __gc AND __close support.
// This allows Lua 5.5's <close> syntax:
//
//	local conn <close> = db.open(...)
//	-- conn:close() called automatically when leaving scope
//
// Also has __gc as a safety net if <close> is not used.
func (L *State) PushCloseableResource(resource Closeable) {
	L.PushUserdata(&luaResource{resource: resource})
	setCloseableResourceMetatable(L)
}

// luaResource wraps a Closeable with closed tracking.
type luaResource struct {
	resource Closeable
	closed   bool
}

func (r *luaResource) close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.resource != nil {
		return r.resource.Close()
	}
	return nil
}

const resourceMetaName = "go.resource"
const closeableResourceMetaName = "go.closeable_resource"

func setResourceMetatable(L *State) {
	if L.NewMetatable(resourceMetaName) {
		// __gc: called by Lua GC when userdata is collected
		L.PushFunction(resourceGC)
		L.SetField(-2, "__gc")
		// __index table with close method
		L.CreateTable(0, 1)
		L.PushFunction(resourceClose)
		L.SetField(-2, "close")
		L.SetField(-2, "__index")
	}
	L.SetMetatable(-2)
}

func setCloseableResourceMetatable(L *State) {
	if L.NewMetatable(closeableResourceMetaName) {
		// __gc: safety net
		L.PushFunction(resourceGC)
		L.SetField(-2, "__gc")
		// __close: for <close> variables (Lua 5.5)
		L.PushFunction(resourceClose)
		L.SetField(-2, "__close")
		// __index table with close method
		L.CreateTable(0, 1)
		L.PushFunction(resourceClose)
		L.SetField(-2, "close")
		L.SetField(-2, "__index")
	}
	L.SetMetatable(-2)
}

// resourceGC is the __gc metamethod — closes the resource.
func resourceGC(L *State) int {
	ud := L.UserdataValue(1)
	if r, ok := ud.(*luaResource); ok {
		r.close() // ignore error in GC
	}
	return 0
}

// resourceClose is the explicit close method + __close metamethod.
// Returns true on success, or nil + error string on failure.
func resourceClose(L *State) int {
	ud := L.UserdataValue(1)
	if r, ok := ud.(*luaResource); ok {
		if err := r.close(); err != nil {
			L.PushNil()
			L.PushString(err.Error())
			return 2
		}
	}
	L.PushBoolean(true)
	return 1
}
