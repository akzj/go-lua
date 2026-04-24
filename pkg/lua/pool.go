package lua

import "sync"

// PoolConfig configures a [StatePool].
type PoolConfig struct {
	// MaxStates is the maximum number of States in the pool.
	// When the pool is full, [StatePool.Put] closes excess States.
	// Default: 8.
	MaxStates int

	// InitFunc is called on each new State after creation.
	// Use it to register modules, set globals, configure sandbox, etc.
	// If nil, States are created with [NewState] defaults.
	InitFunc func(L *State)

	// Sandbox, if non-nil, creates sandboxed States via [NewSandboxState]
	// instead of regular ones.
	Sandbox *SandboxConfig
}

// PoolStats contains statistics about a [StatePool].
type PoolStats struct {
	Available int // States currently idle in the pool.
	Created   int // Total States created over the pool's lifetime.
	MaxStates int // Configured maximum pool size.
}

// StatePool manages a pool of reusable Lua [State] values.
//
// Thread-safe: [StatePool.Get] and [StatePool.Put] can be called from any
// goroutine. Each State returned by Get is exclusively owned by the caller
// until Put is called — States are never shared between goroutines.
type StatePool struct {
	mu      sync.Mutex
	states  []*State
	config  PoolConfig
	created int  // total States created (for stats)
	closed  bool // true after Close()
}

// NewStatePool creates a new State pool with the given configuration.
func NewStatePool(config PoolConfig) *StatePool {
	if config.MaxStates <= 0 {
		config.MaxStates = 8
	}
	return &StatePool{
		config: config,
		states: make([]*State, 0, config.MaxStates),
	}
}

// Get retrieves a State from the pool, or creates a new one if the pool is
// empty. The returned State is exclusively owned by the caller until
// [StatePool.Put] is called.
func (p *StatePool) Get() *State {
	p.mu.Lock()
	if len(p.states) > 0 {
		L := p.states[len(p.states)-1]
		p.states = p.states[:len(p.states)-1]
		p.mu.Unlock()
		return L
	}
	p.created++
	p.mu.Unlock()
	return p.newState()
}

// Put returns a State to the pool for reuse.
// If the pool is full or closed, the State is closed instead.
//
// The caller must not use L after calling Put.
func (p *StatePool) Put(L *State) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || len(p.states) >= p.config.MaxStates {
		L.Close()
		return
	}
	p.states = append(p.states, L)
}

// Close closes all States in the pool and prevents further use.
// After Close, [StatePool.Get] will still create new States, but
// [StatePool.Put] will close them immediately.
func (p *StatePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for _, L := range p.states {
		L.Close()
	}
	p.states = nil
}

// Stats returns a snapshot of pool statistics.
func (p *StatePool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PoolStats{
		Available: len(p.states),
		Created:   p.created,
		MaxStates: p.config.MaxStates,
	}
}

// newState creates a new Lua State using the pool's configuration.
func (p *StatePool) newState() *State {
	var L *State
	if p.config.Sandbox != nil {
		L = NewSandboxState(*p.config.Sandbox)
	} else {
		L = NewState()
	}
	if p.config.InitFunc != nil {
		p.config.InitFunc(L)
	}
	return L
}
