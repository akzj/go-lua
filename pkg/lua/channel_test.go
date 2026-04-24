package lua_test

import (
	"sync"
	"testing"
	"time"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// Go-level Channel tests
// ---------------------------------------------------------------------------

func TestChannel_SendRecv(t *testing.T) {
	ch := lua.NewChannel(1)
	if err := ch.Send("hello"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	val, ok := ch.Recv()
	if !ok {
		t.Fatal("Recv returned ok=false")
	}
	if val != "hello" {
		t.Fatalf("expected 'hello', got %v", val)
	}
}

func TestChannel_Buffered(t *testing.T) {
	ch := lua.NewChannel(3)
	// Fill buffer without blocking.
	for i := 0; i < 3; i++ {
		if err := ch.Send(i); err != nil {
			t.Fatalf("Send(%d) failed: %v", i, err)
		}
	}
	if ch.Len() != 3 {
		t.Fatalf("expected Len()=3, got %d", ch.Len())
	}
	// Drain.
	for i := 0; i < 3; i++ {
		val, ok := ch.Recv()
		if !ok || val != i {
			t.Fatalf("Recv: expected (%d, true), got (%v, %v)", i, val, ok)
		}
	}
}

func TestChannel_Unbuffered(t *testing.T) {
	ch := lua.NewChannel(0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		val, ok := ch.Recv()
		if !ok || val != "sync" {
			t.Errorf("Recv: expected ('sync', true), got (%v, %v)", val, ok)
		}
	}()
	// Give goroutine time to block on Recv.
	time.Sleep(10 * time.Millisecond)
	if err := ch.Send("sync"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	<-done
}

func TestChannel_TrySend(t *testing.T) {
	ch := lua.NewChannel(1)
	// First TrySend should succeed (buffer has space).
	if !ch.TrySend("a") {
		t.Fatal("TrySend should succeed on empty buffer")
	}
	// Second TrySend should fail (buffer full).
	if ch.TrySend("b") {
		t.Fatal("TrySend should fail on full buffer")
	}
}

func TestChannel_TryRecv(t *testing.T) {
	ch := lua.NewChannel(1)
	// TryRecv on empty channel.
	val, gotValue, open := ch.TryRecv()
	if gotValue || !open {
		t.Fatalf("expected (nil, false, true), got (%v, %v, %v)", val, gotValue, open)
	}
	// Send then TryRecv.
	ch.Send("x")
	val, gotValue, open = ch.TryRecv()
	if !gotValue || !open || val != "x" {
		t.Fatalf("expected ('x', true, true), got (%v, %v, %v)", val, gotValue, open)
	}
}

func TestChannel_Close(t *testing.T) {
	ch := lua.NewChannel(1)
	ch.Send("last")
	ch.Close()

	// IsClosed should be true.
	if !ch.IsClosed() {
		t.Fatal("expected IsClosed()=true")
	}
	// Send on closed channel returns error.
	if err := ch.Send("fail"); err != lua.ErrChannelClosed {
		t.Fatalf("expected ErrChannelClosed, got %v", err)
	}
	// TrySend on closed channel returns false.
	if ch.TrySend("fail") {
		t.Fatal("TrySend should return false on closed channel")
	}
	// Can still recv buffered value.
	val, ok := ch.Recv()
	if !ok || val != "last" {
		t.Fatalf("expected ('last', true), got (%v, %v)", val, ok)
	}
	// Recv on closed+empty returns nil, false.
	val, ok = ch.Recv()
	if ok || val != nil {
		t.Fatalf("expected (nil, false), got (%v, %v)", val, ok)
	}
	// Double close is safe.
	ch.Close()
}

func TestChannel_TryRecvClosed(t *testing.T) {
	ch := lua.NewChannel(1)
	ch.Send("buffered")
	ch.Close()

	// TryRecv should get the buffered value.
	val, gotValue, open := ch.TryRecv()
	if !gotValue || val != "buffered" {
		t.Fatalf("expected ('buffered', true, _), got (%v, %v, %v)", val, gotValue, open)
	}

	// TryRecv on closed+empty: gotValue=true (from select case), open=false.
	val, gotValue, open = ch.TryRecv()
	if open {
		t.Fatalf("expected open=false on closed+empty channel, got (%v, %v, %v)", val, gotValue, open)
	}
}

func TestChannel_RecvTimeout(t *testing.T) {
	ch := lua.NewChannel(0)

	// Timeout on empty channel.
	start := time.Now()
	val, ok := ch.RecvTimeout(50 * time.Millisecond)
	elapsed := time.Since(start)
	if ok || val != nil {
		t.Fatalf("expected timeout, got (%v, %v)", val, ok)
	}
	if elapsed < 40*time.Millisecond {
		t.Fatalf("timeout too fast: %v", elapsed)
	}

	// Recv with value available.
	go func() {
		time.Sleep(10 * time.Millisecond)
		ch.Send("delayed")
	}()
	val, ok = ch.RecvTimeout(1 * time.Second)
	if !ok || val != "delayed" {
		t.Fatalf("expected ('delayed', true), got (%v, %v)", val, ok)
	}
}

func TestChannel_CrossGoroutine(t *testing.T) {
	ch := lua.NewChannel(0)
	const N = 100
	var wg sync.WaitGroup
	wg.Add(2)

	// Producer.
	go func() {
		defer wg.Done()
		for i := 0; i < N; i++ {
			if err := ch.Send(i); err != nil {
				t.Errorf("Send(%d) failed: %v", i, err)
				return
			}
		}
		ch.Close()
	}()

	// Consumer.
	received := make([]int, 0, N)
	go func() {
		defer wg.Done()
		for {
			val, ok := ch.Recv()
			if !ok {
				break
			}
			received = append(received, val.(int))
		}
	}()

	wg.Wait()
	if len(received) != N {
		t.Fatalf("expected %d values, got %d", N, len(received))
	}
	for i, v := range received {
		if v != i {
			t.Fatalf("received[%d] = %d, expected %d", i, v, i)
		}
	}
}

// ---------------------------------------------------------------------------
// Lua API tests
// ---------------------------------------------------------------------------

func TestChannel_LuaAPI_Basic(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// Create a buffered channel and send/recv in Lua.
	err := L.DoString(`
		local channel = require("channel")
		local ch = channel.new(5)

		-- Send values.
		assert(channel.send(ch, "hello") == true)
		assert(channel.send(ch, 42) == true)
		assert(channel.send(ch, true) == true)

		-- Recv values.
		local v1, ok1 = channel.recv(ch)
		assert(v1 == "hello", "expected 'hello', got: " .. tostring(v1))
		assert(ok1 == true)

		local v2, ok2 = channel.recv(ch)
		assert(v2 == 42, "expected 42, got: " .. tostring(v2))
		assert(ok2 == true)

		local v3, ok3 = channel.recv(ch)
		assert(v3 == true, "expected true, got: " .. tostring(v3))
		assert(ok3 == true)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestChannel_LuaAPI_TrySend(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local channel = require("channel")
		local ch = channel.new(1)

		-- First try_send should succeed.
		assert(channel.try_send(ch, "a") == true)
		-- Second try_send should fail (buffer full).
		assert(channel.try_send(ch, "b") == false)
		-- Drain.
		local v, ok = channel.recv(ch)
		assert(v == "a" and ok == true)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestChannel_LuaAPI_TryRecv(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local channel = require("channel")
		local ch = channel.new(1)

		-- try_recv on empty channel returns nil, false.
		local v, ok = channel.try_recv(ch)
		assert(v == nil, "expected nil, got: " .. tostring(v))
		assert(ok == false, "expected false, got: " .. tostring(ok))

		-- Send then try_recv.
		channel.send(ch, "data")
		v, ok = channel.try_recv(ch)
		assert(v == "data", "expected 'data', got: " .. tostring(v))
		assert(ok == true, "expected true, got: " .. tostring(ok))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestChannel_LuaAPI_Close(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local channel = require("channel")
		local ch = channel.new(1)
		channel.send(ch, "last")
		channel.close(ch)

		assert(channel.is_closed(ch) == true)

		-- Send on closed channel returns false + error message.
		local ok, msg = channel.send(ch, "fail")
		assert(ok == false, "expected false, got: " .. tostring(ok))
		assert(type(msg) == "string", "expected error string, got: " .. type(msg))

		-- Can still recv buffered value.
		local v, rok = channel.recv(ch)
		assert(v == "last", "expected 'last', got: " .. tostring(v))
		assert(rok == true)

		-- Recv on closed+empty returns nil, false.
		v, rok = channel.recv(ch)
		assert(v == nil, "expected nil, got: " .. tostring(v))
		assert(rok == false)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestChannel_LuaAPI_Len(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local channel = require("channel")
		local ch = channel.new(5)
		assert(channel.len(ch) == 0)
		channel.send(ch, 1)
		channel.send(ch, 2)
		assert(channel.len(ch) == 2)
		channel.recv(ch)
		assert(channel.len(ch) == 1)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestChannel_LuaAPI_TryRecvClosed(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local channel = require("channel")
		local ch = channel.new(0)
		channel.close(ch)

		-- try_recv on closed+empty returns nil, false, "closed".
		local v, ok, reason = channel.try_recv(ch)
		assert(v == nil, "expected nil, got: " .. tostring(v))
		assert(ok == false, "expected false, got: " .. tostring(ok))
		assert(reason == "closed", "expected 'closed', got: " .. tostring(reason))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestChannel_LuaAPI_DefaultUnbuffered(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// channel.new() with no args should create unbuffered channel.
	err := L.DoString(`
		local channel = require("channel")
		local ch = channel.new()
		assert(channel.len(ch) == 0)
		-- try_send on unbuffered should fail (no receiver).
		assert(channel.try_send(ch, "x") == false)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Cross-boundary tests (Go ↔ Lua)
// ---------------------------------------------------------------------------

func TestChannel_GoToLua(t *testing.T) {
	ch := lua.NewChannel(1)

	L := lua.NewState()
	defer L.Close()

	// Push the channel as a global.
	L.PushUserdata(ch)
	L.SetGlobal("ch")

	// Go sends, Lua receives.
	ch.Send("from-go")

	err := L.DoString(`
		local channel = require("channel")
		local v, ok = channel.recv(ch)
		assert(v == "from-go", "expected 'from-go', got: " .. tostring(v))
		assert(ok == true)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestChannel_LuaToGo(t *testing.T) {
	ch := lua.NewChannel(1)

	L := lua.NewState()
	defer L.Close()

	// Push the channel as a global.
	L.PushUserdata(ch)
	L.SetGlobal("ch")

	// Lua sends, Go receives.
	err := L.DoString(`
		local channel = require("channel")
		channel.send(ch, "from-lua")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	val, ok := ch.Recv()
	if !ok || val != "from-lua" {
		t.Fatalf("expected ('from-lua', true), got (%v, %v)", val, ok)
	}
}

func TestChannel_CrossLuaStates(t *testing.T) {
	ch := lua.NewChannel(0) // unbuffered for synchronous handoff
	var wg sync.WaitGroup
	wg.Add(2)

	// State 1: sender (in goroutine).
	go func() {
		defer wg.Done()
		L1 := lua.NewState()
		defer L1.Close()

		L1.PushUserdata(ch)
		L1.SetGlobal("ch")

		err := L1.DoString(`
			local channel = require("channel")
			channel.send(ch, "cross-state")
		`)
		if err != nil {
			t.Errorf("L1 DoString failed: %v", err)
		}
	}()

	// State 2: receiver (in goroutine).
	go func() {
		defer wg.Done()
		L2 := lua.NewState()
		defer L2.Close()

		L2.PushUserdata(ch)
		L2.SetGlobal("ch")

		err := L2.DoString(`
			local channel = require("channel")
			local v, ok = channel.recv(ch)
			assert(v == "cross-state", "expected 'cross-state', got: " .. tostring(v))
			assert(ok == true)
		`)
		if err != nil {
			t.Errorf("L2 DoString failed: %v", err)
		}
	}()

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Race detector test
// ---------------------------------------------------------------------------

func TestChannel_RaceDetector(t *testing.T) {
	ch := lua.NewChannel(10)
	const goroutines = 8
	const msgsPerGoroutine = 50
	var wg sync.WaitGroup

	// Producers.
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < msgsPerGoroutine; i++ {
				ch.Send(id*1000 + i)
			}
		}(g)
	}

	// Consumer.
	received := make(chan int, goroutines*msgsPerGoroutine)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < goroutines*msgsPerGoroutine; i++ {
			val, ok := ch.Recv()
			if !ok {
				t.Errorf("Recv returned ok=false at i=%d", i)
				return
			}
			received <- val.(int)
		}
	}()

	wg.Wait()
	close(received)

	count := 0
	for range received {
		count++
	}
	if count != goroutines*msgsPerGoroutine {
		t.Fatalf("expected %d messages, got %d", goroutines*msgsPerGoroutine, count)
	}
}

func TestChannel_ConcurrentCloseAndSend(t *testing.T) {
	ch := lua.NewChannel(0)
	var wg sync.WaitGroup

	// Multiple goroutines try to send while another closes.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// TrySend won't panic on closed channel.
			for j := 0; j < 100; j++ {
				ch.TrySend(j)
			}
		}()
	}

	// Close after a brief delay.
	time.Sleep(5 * time.Millisecond)
	ch.Close()
	wg.Wait()

	// Verify channel is closed.
	if !ch.IsClosed() {
		t.Fatal("expected IsClosed()=true")
	}
}

func TestChannel_ConcurrentCloseAndRecv(t *testing.T) {
	ch := lua.NewChannel(10)
	// Fill buffer.
	for i := 0; i < 10; i++ {
		ch.Send(i)
	}
	ch.Close()

	// Multiple goroutines drain concurrently.
	var wg sync.WaitGroup
	total := make(chan int, 10)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count := 0
			for {
				_, ok := ch.Recv()
				if !ok {
					break
				}
				count++
			}
			total <- count
		}()
	}
	wg.Wait()
	close(total)

	sum := 0
	for c := range total {
		sum += c
	}
	if sum != 10 {
		t.Fatalf("expected 10 total received, got %d", sum)
	}
}
