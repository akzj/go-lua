package lmem

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lobject"
)

/*
** Maximum size_t
 */
const MAX_SIZET = ^uint64(0)

/*
** luaM_error - raise memory error
** In Go, we use panic to simulate setjmp/longjmp behavior
 */
func Error(L *lobject.LuaState) {
	panic(LuaErrMem)
}

/*
** Memory error types
 */
type LuaError int

const (
	LuaErrMem LuaError = iota
)

/*
** Default memory allocator
 */
func DefaultAlloc(ud interface{}, ptr interface{}, osize uint64, nsize uint64) interface{} {
	if nsize == 0 {
		return nil
	}
	if ptr == nil {
		return make([]byte, nsize)
	}
	old := ptr.([]byte)
	if uint64(len(old)) >= nsize {
		return old[:nsize]
	}
	result := make([]byte, nsize)
	copy(result, old)
	return result
}

/*
** Allocate object with GC header
 */
func NewObject(L *lobject.LuaState, size uint64, tt lobject.LuByte) interface{} {
	return Malloc(L, size)
}

/*
** Allocate memory
 */
func Malloc(L *lobject.LuaState, size uint64) interface{} {
	if size == 0 {
		return nil
	}
	return make([]byte, size)
}

/*
** Free memory
 */
func Free(L *lobject.LuaState, block interface{}) {
	// Go GC handles this
}

/*
** Reallocate memory
 */
func Realloc(L *lobject.LuaState, old interface{}, oldsize, newsize uint64) interface{} {
	if old == nil {
		return Malloc(L, newsize)
	}
	if newsize == 0 {
		return nil
	}
	oldBytes := old.([]byte)
	if uint64(len(oldBytes)) >= newsize {
		return oldBytes[:newsize]
	}
	newBlock := make([]byte, newsize)
	copy(newBlock, oldBytes)
	return newBlock
}

/*
** Allocate vector
 */
func NewVector(L *lobject.LuaState, n uint64, t interface{}) interface{} {
	size := n * uint64(unsafe.Sizeof(t))
	return Malloc(L, size)
}

/*
** Free vector
 */
func FreeVector(L *lobject.LuaState, v interface{}) {
	Free(L, v)
}

/*
** Reallocate vector
 */
func ReallocVector(L *lobject.LuaState, v interface{}, oldn, n uint64) interface{} {
	oldsize := oldn * uint64(unsafe.Sizeof(v))
	newsize := n * uint64(unsafe.Sizeof(v))
	return Realloc(L, v, oldsize, newsize)
}

/*
** Grow vector
 */
func GrowVector(L *lobject.LuaState, v interface{}, nelems *int, size *int, t interface{}, limit int, what string) interface{} {
	if *nelems+1 <= *size {
		return v
	}
	if *size >= limit/2 {
		if *size >= limit {
			return v
		}
		*size = limit
	} else {
		*size *= 2
		if *size < 4 {
			*size = 4
		}
	}
	return ReallocVector(L, v, uint64(*nelems), uint64(*size))
}

/*
** Check memory allocation limit
 */
func LimitN(n, t uint64) uint64 {
	maxSize := MAX_SIZET / t
	if n <= maxSize {
		return n
	}
	return maxSize
}

/*
** Size tests
 */
func TestSize(n, e uint64) bool {
	return n+1 > MAX_SIZET/e
}

/*
** Check size overflow
 */
func CheckSize(L *lobject.LuaState, n, e uint64) {
	if TestSize(n, e) {
		Error(L)
	}
}
