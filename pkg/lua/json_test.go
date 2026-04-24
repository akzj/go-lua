package lua_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/lua"
)

// ---------------------------------------------------------------------------
// json.encode tests
// ---------------------------------------------------------------------------

func TestJSON_EncodeTable(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		local s = json.encode({name = "Alice", age = 30})
		-- Decode in Go to verify (order may vary).
		_G._result = s
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	L.GetGlobal("_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	// Parse the JSON to verify content (key order may vary).
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON output: %s — error: %v", result, err)
	}
	if m["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", m["name"])
	}
	// age may be float64 from Go's json.Unmarshal.
	if age, ok := m["age"].(float64); !ok || age != 30 {
		t.Errorf("expected age=30, got %v", m["age"])
	}
}

func TestJSON_EncodeArray(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		_G._result = json.encode({1, 2, 3})
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	L.GetGlobal("_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	if result != "[1,2,3]" {
		t.Errorf("expected [1,2,3], got %s", result)
	}
}

func TestJSON_EncodeString(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		_G._result = json.encode("hello")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	L.GetGlobal("_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	if result != `"hello"` {
		t.Errorf(`expected "hello", got %s`, result)
	}
}

func TestJSON_EncodeNumber(t *testing.T) {
	L := lua.NewState()
	// Test integer.
	err := L.DoString(`
		local json = require("json")
		_G._int_result = json.encode(42)
		_G._float_result = json.encode(3.14)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("_int_result")
	intResult, _ := L.ToString(-1)
	L.Pop(1)
	if intResult != "42" {
		t.Errorf("expected 42, got %s", intResult)
	}

	L.GetGlobal("_float_result")
	floatResult, _ := L.ToString(-1)
	L.Pop(1)
	if floatResult != "3.14" {
		t.Errorf("expected 3.14, got %s", floatResult)
	}
}

func TestJSON_EncodeBoolean(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		_G._true = json.encode(true)
		_G._false = json.encode(false)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	L.GetGlobal("_true")
	trueResult, _ := L.ToString(-1)
	L.Pop(1)
	if trueResult != "true" {
		t.Errorf("expected true, got %s", trueResult)
	}

	L.GetGlobal("_false")
	falseResult, _ := L.ToString(-1)
	L.Pop(1)
	if falseResult != "false" {
		t.Errorf("expected false, got %s", falseResult)
	}
}

func TestJSON_EncodeNil(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		_G._result = json.encode(nil)
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	L.GetGlobal("_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	if result != "null" {
		t.Errorf("expected null, got %s", result)
	}
}

func TestJSON_EncodeNested(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		_G._result = json.encode({
			users = {
				{name = "Alice", age = 30},
				{name = "Bob", age = 25},
			}
		})
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	L.GetGlobal("_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	// Parse to verify structure.
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	users, ok := m["users"].([]any)
	if !ok || len(users) != 2 {
		t.Fatalf("expected users array of length 2, got %v", m["users"])
	}
	user0, ok := users[0].(map[string]any)
	if !ok || user0["name"] != "Alice" {
		t.Errorf("expected first user Alice, got %v", users[0])
	}
}

func TestJSON_EncodeEmptyTable(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		_G._result = json.encode({})
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	L.GetGlobal("_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	// Empty table → empty object (ToAny returns map[string]any{}).
	if result != "{}" {
		t.Errorf("expected {}, got %s", result)
	}
}

// ---------------------------------------------------------------------------
// json.decode tests
// ---------------------------------------------------------------------------

func TestJSON_DecodeObject(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		local t = json.decode('{"name":"Alice","age":30}')
		assert(t.name == "Alice", "name mismatch: " .. tostring(t.name))
		assert(t.age == 30, "age mismatch: " .. tostring(t.age))
		assert(type(t.age) == "number", "age should be number")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_DecodeArray(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		local arr = json.decode('[1, 2, 3]')
		assert(arr[1] == 1, "arr[1] mismatch")
		assert(arr[2] == 2, "arr[2] mismatch")
		assert(arr[3] == 3, "arr[3] mismatch")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_DecodeString(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		local s = json.decode('"hello world"')
		assert(s == "hello world", "expected 'hello world', got: " .. tostring(s))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_DecodeNumber(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		-- Integer
		local n = json.decode('42')
		assert(n == 42, "expected 42, got " .. tostring(n))
		assert(math.type(n) == "integer", "expected integer, got " .. math.type(n))
		-- Float
		local f = json.decode('3.14')
		assert(f == 3.14, "expected 3.14, got " .. tostring(f))
		assert(math.type(f) == "float", "expected float, got " .. math.type(f))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_DecodeBoolean(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		assert(json.decode('true') == true, "expected true")
		assert(json.decode('false') == false, "expected false")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_DecodeNull(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		local v = json.decode('null')
		assert(v == nil, "expected nil, got " .. tostring(v))
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_DecodeNested(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		local data = json.decode('{"users":[{"name":"Alice"},{"name":"Bob"}]}')
		assert(data.users[1].name == "Alice", "first user should be Alice")
		assert(data.users[2].name == "Bob", "second user should be Bob")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_DecodeInvalid(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		local ok, err = pcall(json.decode, "not valid json{{{")
		assert(not ok, "expected error for invalid JSON")
		assert(type(err) == "string", "error should be a string")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// json.encode_pretty tests
// ---------------------------------------------------------------------------

func TestJSON_EncodePretty(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		_G._result = json.encode_pretty({name = "Alice"})
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	L.GetGlobal("_result")
	result, _ := L.ToString(-1)
	L.Pop(1)

	// Should have indentation.
	if !strings.Contains(result, "\n") {
		t.Errorf("expected pretty-printed JSON with newlines, got: %s", result)
	}
	if !strings.Contains(result, "  ") {
		t.Errorf("expected 2-space indentation, got: %s", result)
	}

	// Verify it's valid JSON.
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("pretty JSON is invalid: %v", err)
	}
	if m["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", m["name"])
	}
}

// ---------------------------------------------------------------------------
// Round-trip tests
// ---------------------------------------------------------------------------

func TestJSON_RoundTrip(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")

		-- Object round-trip.
		local obj = {name = "Alice", age = 30, active = true}
		local s = json.encode(obj)
		local obj2 = json.decode(s)
		assert(obj2.name == "Alice", "name mismatch after round-trip")
		assert(obj2.age == 30, "age mismatch after round-trip")
		assert(obj2.active == true, "active mismatch after round-trip")

		-- Array round-trip.
		local arr = {10, 20, 30}
		local s2 = json.encode(arr)
		local arr2 = json.decode(s2)
		assert(arr2[1] == 10, "arr[1] mismatch")
		assert(arr2[2] == 20, "arr[2] mismatch")
		assert(arr2[3] == 30, "arr[3] mismatch")

		-- Scalar round-trips.
		assert(json.decode(json.encode("hello")) == "hello", "string round-trip failed")
		assert(json.decode(json.encode(42)) == 42, "integer round-trip failed")
		assert(json.decode(json.encode(3.14)) == 3.14, "float round-trip failed")
		assert(json.decode(json.encode(true)) == true, "bool round-trip failed")
		assert(json.decode(json.encode(false)) == false, "bool false round-trip failed")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_RoundTripNested(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		local data = {
			users = {
				{name = "Alice", scores = {100, 95, 88}},
				{name = "Bob", scores = {70, 80}},
			},
			count = 2,
		}
		local s = json.encode(data)
		local data2 = json.decode(s)
		assert(data2.count == 2, "count mismatch")
		assert(data2.users[1].name == "Alice", "first user name mismatch")
		assert(data2.users[1].scores[1] == 100, "first score mismatch")
		assert(data2.users[2].name == "Bob", "second user name mismatch")
		assert(#data2.users[2].scores == 2, "Bob scores count mismatch")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// require("json") tests
// ---------------------------------------------------------------------------

func TestJSON_RequireModule(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		assert(type(json) == "table", "json should be a table")
		assert(type(json.encode) == "function", "json.encode should be a function")
		assert(type(json.decode) == "function", "json.decode should be a function")
		assert(type(json.encode_pretty) == "function", "json.encode_pretty should be a function")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_RequireCached(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json1 = require("json")
		local json2 = require("json")
		assert(json1 == json2, "require should return cached module")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Large integer precision test
// ---------------------------------------------------------------------------

func TestJSON_LargeInteger(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")
		-- 2^53 + 1 = 9007199254740993 — exceeds float64 precision.
		local big = 9007199254740993
		local s = json.encode(big)
		assert(s == "9007199254740993", "large int should be exact, got: " .. s)
		local decoded = json.decode(s)
		assert(decoded == big, "large int round-trip failed")
		assert(math.type(decoded) == "integer", "should remain integer")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Full Lua integration test
// ---------------------------------------------------------------------------

func TestJSON_LuaIntegration(t *testing.T) {
	L := lua.NewState()
	err := L.DoString(`
		local json = require("json")

		-- Build a config table.
		local config = {
			host = "localhost",
			port = 8080,
			debug = false,
			tags = {"web", "api"},
		}

		-- Encode to JSON.
		local s = json.encode(config)
		assert(type(s) == "string", "encode should return string")

		-- Decode back.
		local cfg = json.decode(s)
		assert(cfg.host == "localhost", "host mismatch")
		assert(cfg.port == 8080, "port mismatch")
		assert(cfg.debug == false, "debug mismatch")
		assert(cfg.tags[1] == "web", "tag 1 mismatch")
		assert(cfg.tags[2] == "api", "tag 2 mismatch")

		-- Pretty print.
		local pretty = json.encode_pretty(cfg)
		assert(type(pretty) == "string", "encode_pretty should return string")

		-- Error handling.
		local ok, err = pcall(json.decode, "}{invalid")
		assert(not ok, "should error on invalid JSON")
	`)
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
}

func TestJSON_MultipleStates(t *testing.T) {
	// Verify that the global registration works across multiple states.
	for i := 0; i < 3; i++ {
		L := lua.NewState()
		err := L.DoString(`
			local json = require("json")
			assert(json.encode(42) == "42", "encode failed in state")
		`)
		if err != nil {
			t.Fatalf("state %d: DoString failed: %v", i, err)
		}
	}
}
