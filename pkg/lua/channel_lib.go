package lua

// OpenChannelLib opens the "channel" module and pushes it onto the stack.
// It is registered globally via init(), so `require("channel")` works
// automatically in any State.
//
// Lua API:
//
//	local channel = require("channel")
//	local ch = channel.new(10)           -- buffered channel, size 10
//	channel.send(ch, "hello")            -- send (blocks if full)
//	local val, ok = channel.recv(ch)     -- receive (blocks if empty)
//	local ok = channel.try_send(ch, val) -- non-blocking send
//	local val, ok = channel.try_recv(ch) -- non-blocking receive
//	channel.close(ch)                    -- close channel
//	local n = channel.len(ch)            -- buffer length
//	local b = channel.is_closed(ch)      -- check if closed
func OpenChannelLib(L *State) {
	L.NewLib(map[string]Function{
		"new":       channelNew,
		"send":      channelSend,
		"recv":      channelRecv,
		"try_send":  channelTrySend,
		"try_recv":  channelTryRecv,
		"close":     channelClose,
		"len":       channelLen,
		"is_closed": channelIsClosed,
	})
}

func init() {
	RegisterGlobal("channel", OpenChannelLib)
}

// checkChannel extracts a *Channel from userdata at the given stack position.
// Raises a Lua error if the argument is not a Channel userdata.
func checkChannel(L *State, idx int) *Channel {
	ud := L.UserdataValue(idx)
	if ud == nil {
		L.ArgError(idx, "channel expected, got nil")
		return nil // unreachable
	}
	ch, ok := ud.(*Channel)
	if !ok {
		L.ArgError(idx, "channel expected")
		return nil // unreachable
	}
	return ch
}

// channelNew creates a new channel.
// Lua: channel.new([bufsize]) → userdata
func channelNew(L *State) int {
	bufSize := int64(0)
	if L.GetTop() >= 1 && !L.IsNoneOrNil(1) {
		bufSize = L.CheckInteger(1)
		if bufSize < 0 {
			L.ArgError(1, "buffer size must be non-negative")
		}
	}
	ch := NewChannel(int(bufSize))
	L.PushUserdata(ch)
	return 1
}

// channelSend sends a value on the channel (blocking).
// Lua: channel.send(ch, value) → true | false, errmsg
func channelSend(L *State) int {
	ch := checkChannel(L, 1)
	val := L.ToAny(2)
	err := ch.Send(val)
	if err != nil {
		L.PushBoolean(false)
		L.PushString(err.Error())
		return 2
	}
	L.PushBoolean(true)
	return 1
}

// channelRecv receives a value from the channel (blocking).
// Lua: channel.recv(ch) → value, true | nil, false
func channelRecv(L *State) int {
	ch := checkChannel(L, 1)
	val, ok := ch.Recv()
	if !ok {
		L.PushNil()
		L.PushBoolean(false)
		return 2
	}
	L.PushAny(val)
	L.PushBoolean(true)
	return 2
}

// channelTrySend attempts a non-blocking send.
// Lua: channel.try_send(ch, value) → true | false
func channelTrySend(L *State) int {
	ch := checkChannel(L, 1)
	val := L.ToAny(2)
	ok := ch.TrySend(val)
	L.PushBoolean(ok)
	return 1
}

// channelTryRecv attempts a non-blocking receive.
// Lua: channel.try_recv(ch) → value, true | nil, false [, "closed"]
// Returns: (value, true) if received; (nil, false) if empty;
// (nil, false, "closed") if channel is closed and drained.
func channelTryRecv(L *State) int {
	ch := checkChannel(L, 1)
	val, gotSelect, open := ch.TryRecv()
	if gotSelect && open {
		// Received a real value from an open channel.
		L.PushAny(val)
		L.PushBoolean(true)
		return 2
	}
	if gotSelect && !open {
		// Select fired because channel is closed (zero value, ok=false).
		L.PushNil()
		L.PushBoolean(false)
		L.PushString("closed")
		return 3
	}
	// Default branch: channel is open but empty.
	L.PushNil()
	L.PushBoolean(false)
	return 2
}

// channelClose closes the channel.
// Lua: channel.close(ch)
func channelClose(L *State) int {
	ch := checkChannel(L, 1)
	ch.Close()
	return 0
}

// channelLen returns the number of buffered elements.
// Lua: channel.len(ch) → integer
func channelLen(L *State) int {
	ch := checkChannel(L, 1)
	L.PushInteger(int64(ch.Len()))
	return 1
}

// channelIsClosed returns whether the channel is closed.
// Lua: channel.is_closed(ch) → boolean
func channelIsClosed(L *State) int {
	ch := checkChannel(L, 1)
	L.PushBoolean(ch.IsClosed())
	return 1
}
