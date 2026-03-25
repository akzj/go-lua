// Package api provides the public Lua API
package api

import (
	"fmt"
	"runtime"
	"strings"
)

// LuaError represents a Lua runtime error.
//
// This error type is used for all Lua runtime errors, including:
//   - Syntax errors during code loading
//   - Runtime errors during execution
//   - Errors raised by lua_error or pcall
//
// The error includes a stack trace showing the call chain at the time
// the error occurred.
type LuaError struct {
	// Message is the error message
	Message string

	// Stack is the Lua stack trace at the time of error
	Stack []string

	// GoStack is the Go stack trace (for debugging)
	GoStack string
}

// Error returns the error message.
//
// This implements the error interface.
func (e *LuaError) Error() string {
	if len(e.Stack) == 0 {
		return e.Message
	}
	return fmt.Sprintf("%s\nstack traceback:\n%s", e.Message, strings.Join(e.Stack, "\n"))
}

// newLuaError creates a new LuaError with stack trace.
//
// Parameters:
//   - message: The error message
//
// Returns:
//   - *LuaError: A new LuaError with stack trace
func newLuaError(message string) *LuaError {
	return &LuaError{
		Message: message,
		Stack:   captureStack(2), // Skip this function and the caller
		GoStack: captureGoStack(),
	}
}

// captureStack captures the Lua stack trace.
//
// This walks the VM's call info stack to build a human-readable
// stack trace showing function names and line numbers.
//
// Parameters:
//   - skip: Number of stack frames to skip
//
// Returns:
//   - []string: Stack trace entries
func captureStack(skip int) []string {
	// For now, return a simple stack trace
	// In a full implementation, this would walk the VM's call info
	return []string{
		fmt.Sprintf("[G]:%d: in function ?", skip),
	}
}

// captureGoStack captures the Go stack trace for debugging.
//
// Returns:
//   - string: Go stack trace
func captureGoStack() string {
	buf := make([]byte, 1024)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// wrapError wraps an error as a LuaError if it isn't already.
//
// Parameters:
//   - err: The error to wrap
//
// Returns:
//   - error: A LuaError or the original error if nil
func wrapError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*LuaError); ok {
		return err
	}
	return newLuaError(err.Error())
}

// LuaErrorType represents the type of Lua error.
type LuaErrorType int

const (
	// ErrSyntax indicates a syntax error
	ErrSyntax LuaErrorType = iota

	// ErrRuntime indicates a runtime error
	ErrRuntime

	// ErrMemory indicates a memory allocation error
	ErrMemory

	// ErrFile indicates a file I/O error
	ErrFile
)

// String returns a human-readable name for the error type.
func (t LuaErrorType) String() string {
	switch t {
	case ErrSyntax:
		return "syntax error"
	case ErrRuntime:
		return "runtime error"
	case ErrMemory:
		return "memory allocation error"
	case ErrFile:
		return "file error"
	default:
		return "unknown error"
	}
}

// SyntaxError creates a new syntax error.
//
// Parameters:
//   - message: The error message
//   - source: The source name (file or string)
//   - line: The line number where the error occurred
//
// Returns:
//   - *LuaError: A new syntax error
func SyntaxError(message, source string, line int) *LuaError {
	return newLuaError(fmt.Sprintf("%s:%d: %s", source, line, message))
}

// RuntimeError creates a new runtime error.
//
// Parameters:
//   - message: The error message
//
// Returns:
//   - *LuaError: A new runtime error
func RuntimeError(message string) *LuaError {
	return newLuaError(message)
}

// FileError creates a new file error.
//
// Parameters:
//   - filename: The file name
//   - err: The underlying error
//
// Returns:
//   - *LuaError: A new file error
func FileError(filename string, err error) *LuaError {
	return newLuaError(fmt.Sprintf("unable to open %s: %v", filename, err))
}