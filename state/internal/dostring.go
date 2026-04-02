// Package internal provides concrete implementation of DoString.
package internal

import (
	"sync"

	bcapi "github.com/akzj/go-lua/bytecode/api"
	stateapi "github.com/akzj/go-lua/state/api"
	"github.com/akzj/go-lua/types"
)

// doStringClosureRegistry stores prototypes for DoString closures.
var (
	doStringRegistryLock sync.Mutex
	doStringRegistry     = make(map[int]bcapi.Prototype)
	doStringNextID       = 1
)

// RegisterDoStringClosure stores the prototype and pushes a marker closure onto the stack.
// The marker is a light userdata that encodes the registry ID.
func RegisterDoStringClosure(L stateapi.LuaStateInterface, proto bcapi.Prototype) {
	// Get concrete LuaState
	luaState, ok := L.(*LuaState)
	if !ok {
		return
	}
	doStringRegistryLock.Lock()
	id := doStringNextID
	doStringNextID++
	doStringRegistry[id] = proto
	doStringRegistryLock.Unlock()

	// Create a marker using the types package
	marker := types.NewDoStringMarker(id)

	// Push onto stack
	luaState.GrowStack(1)
	luaState.SetTop(luaState.Top() + 1)
	stack := luaState.Stack()
	stack[luaState.Top()-1] = marker
}

// lookupDoStringPrototype looks up a prototype by the marker value.
func lookupDoStringPrototype(data interface{}) bcapi.Prototype {
	id, ok := data.(int)
	if !ok {
		return nil
	}

	doStringRegistryLock.Lock()
	defer doStringRegistryLock.Unlock()

	return doStringRegistry[id]
}

// IsDoStringMarker checks if a TValue is a DoString marker.
func IsDoStringMarker(tv interface{ IsLightUserData() bool }) bool {
	return tv.IsLightUserData()
}

// GetDoStringMarkerID extracts the registry ID from a DoString marker.
func GetDoStringMarkerID(tv interface{ IsLightUserData() bool }) int {
	if !tv.IsLightUserData() {
		return 0
	}
	// The marker stores int in its data
	// This is a stub - actual implementation needs access to internal types
	return 0
}
