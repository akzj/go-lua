package lua

import (
	"fmt"
	"sync"
	"time"
)

// Channel is a thread-safe communication channel between Lua States
// or between Go and Lua. It wraps a Go channel with Lua-friendly API.
//
// Channels carry any values; when used from Lua, values are converted
// via the PushAny/ToAny bridge automatically.
//
// A Channel can be shared across goroutines and across Lua States safely
// because the underlying Go chan is inherently thread-safe, and the closed
// flag is protected by a RWMutex.
type Channel struct {
	ch     chan any
	closed bool
	mu     sync.RWMutex // protects closed flag
}

// NewChannel creates a new Channel with the given buffer size.
// bufSize 0 creates an unbuffered (synchronous) channel.
func NewChannel(bufSize int) *Channel {
	return &Channel{
		ch: make(chan any, bufSize),
	}
}

// Send sends a value into the channel. Blocks until received (if unbuffered)
// or until buffer has space.
// Returns ErrChannelClosed if the channel has been closed.
//
// Note: Send does NOT hold the mutex during the blocking channel send
// because that would deadlock with Close on unbuffered/full channels.
// Instead it checks the closed flag first, then uses recover to catch
// any panic from sending on a concurrently-closed Go channel.
func (c *Channel) Send(value any) (retErr error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return ErrChannelClosed
	}
	c.mu.RUnlock()

	// Recover from panic if channel is closed between our check and send.
	defer func() {
		if r := recover(); r != nil {
			retErr = ErrChannelClosed
		}
	}()
	c.ch <- value
	return nil
}

// TrySend attempts to send without blocking.
// Returns true if the value was sent, false if the channel is full or closed.
// Safe to call concurrently with Close.
func (c *Channel) TrySend(value any) (sent bool) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return false
	}
	// Hold the read lock during the select so Close() cannot close the Go
	// channel between our closed-flag check and the actual send.
	// This is safe because select with default never blocks, so the lock
	// is held only briefly.
	defer c.mu.RUnlock()
	select {
	case c.ch <- value:
		return true
	default:
		return false
	}
}

// Recv receives a value from the channel. Blocks until a value is available.
// Returns (value, true) on success, or (nil, false) if the channel is closed
// and empty.
func (c *Channel) Recv() (any, bool) {
	val, ok := <-c.ch
	return val, ok
}

// TryRecv attempts to receive without blocking.
// Returns (value, true, true) if a value was received.
// Returns (nil, true, false) if the channel is closed and drained.
// Returns (nil, false, true) if the channel is open but empty.
func (c *Channel) TryRecv() (any, bool, bool) {
	select {
	case val, ok := <-c.ch:
		return val, true, ok
	default:
		return nil, false, true // open but empty
	}
}

// RecvTimeout receives with a timeout.
// Returns (value, true) if received; (nil, false) if timeout or closed.
func (c *Channel) RecvTimeout(timeout time.Duration) (any, bool) {
	select {
	case val, ok := <-c.ch:
		if !ok {
			return nil, false
		}
		return val, true
	case <-time.After(timeout):
		return nil, false
	}
}

// Close closes the channel. Further sends will return ErrChannelClosed.
// Closing an already-closed channel is a no-op.
func (c *Channel) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.ch)
	}
}

// Len returns the number of elements currently buffered in the channel.
func (c *Channel) Len() int {
	return len(c.ch)
}

// IsClosed returns whether the channel has been closed.
func (c *Channel) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// ErrChannelClosed is returned when attempting to send on a closed channel.
var ErrChannelClosed = fmt.Errorf("channel is closed")
