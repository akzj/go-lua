package lua

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

func init() {
	RegisterGlobal("json", OpenJSON)
}

// OpenJSON opens the "json" module and pushes it onto the stack.
// It is registered globally via init(), so `require("json")` works
// automatically in any State.
//
// Lua API:
//
//	local json = require("json")
//	local s = json.encode({name = "Alice", age = 30})
//	local t = json.decode('{"name":"Alice","age":30}')
//	local s = json.encode_pretty({name = "Alice"})
func OpenJSON(L *State) {
	L.NewLib(map[string]Function{
		"encode":        jsonEncode,
		"decode":        jsonDecode,
		"encode_pretty": jsonEncodePretty,
	})
}

// jsonEncode encodes a Lua value to a JSON string.
// Lua: json.encode(value) → string
//
// Supports tables (arrays and objects), strings, numbers, booleans, and nil.
// Returns the JSON string on success, or raises a Lua error on failure.
func jsonEncode(L *State) int {
	return jsonEncodeImpl(L, false)
}

// jsonEncodePretty encodes a Lua value to a pretty-printed JSON string.
// Lua: json.encode_pretty(value) → string
func jsonEncodePretty(L *State) int {
	return jsonEncodeImpl(L, true)
}

func jsonEncodeImpl(L *State, pretty bool) int {
	// Accept 0 args (encode nil) or 1 arg.
	var goVal any
	if L.GetTop() < 1 || L.IsNoneOrNil(1) {
		goVal = nil
	} else {
		goVal = L.ToAny(1)
	}

	// Convert int64 values to json.Number to preserve large integers,
	// and recursively process maps/slices to sort map keys for deterministic output.
	goVal = prepareForJSON(goVal)

	var data []byte
	var err error
	if pretty {
		data, err = json.MarshalIndent(goVal, "", "  ")
	} else {
		data, err = json.Marshal(goVal)
	}
	if err != nil {
		L.Errorf("json.encode: %s", err.Error())
		return 0 // unreachable
	}

	L.PushString(string(data))
	return 1
}

// prepareForJSON recursively converts Go values for JSON marshaling:
// - int64 → json.Number (preserves large integers)
// - map keys are sorted for deterministic output
func prepareForJSON(v any) any {
	switch val := v.(type) {
	case int64:
		// Use json.Number to avoid float64 precision loss for large ints.
		return json.Number(strconv.FormatInt(val, 10))
	case float64:
		// Check for special float values that JSON doesn't support.
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil
		}
		return val
	case map[string]any:
		m := make(map[string]any, len(val))
		for k, v2 := range val {
			m[k] = prepareForJSON(v2)
		}
		return m
	case []any:
		s := make([]any, len(val))
		for i, v2 := range val {
			s[i] = prepareForJSON(v2)
		}
		return s
	default:
		return v
	}
}

// jsonDecode decodes a JSON string to a Lua value.
// Lua: json.decode(str) → value
//
// JSON objects → Lua tables (string keys)
// JSON arrays → Lua tables (integer keys starting at 1)
// JSON strings → Lua strings
// JSON numbers → Lua integers (if no fractional part) or Lua numbers (float)
// JSON booleans → Lua booleans
// JSON null → Lua nil
//
// Returns the decoded value on success, or raises a Lua error on failure.
func jsonDecode(L *State) int {
	str := L.CheckString(1)

	// Use json.Decoder with UseNumber to preserve integer precision.
	dec := json.NewDecoder(strings.NewReader(str))
	dec.UseNumber()

	var goVal any
	if err := dec.Decode(&goVal); err != nil {
		L.Errorf("json.decode: %s", err.Error())
		return 0 // unreachable
	}

	// Convert json.Number to int64 or float64, recursively.
	goVal = convertJSONNumbers(goVal)

	L.PushAny(goVal)
	return 1
}

// convertJSONNumbers recursively converts json.Number values to int64 or float64.
// If the number has no fractional part and fits in int64, it becomes int64.
// Otherwise it becomes float64.
func convertJSONNumbers(v any) any {
	switch val := v.(type) {
	case json.Number:
		// Try integer first.
		if i, err := val.Int64(); err == nil {
			// Verify it round-trips (handles cases like "1.0" which Int64 would reject).
			return i
		}
		// Fall back to float64.
		if f, err := val.Float64(); err == nil {
			return f
		}
		// Shouldn't happen with valid JSON, but return the string.
		return val.String()
	case map[string]any:
		m := make(map[string]any, len(val))
		for k, v2 := range val {
			m[k] = convertJSONNumbers(v2)
		}
		return m
	case []any:
		s := make([]any, len(val))
		for i, v2 := range val {
			s[i] = convertJSONNumbers(v2)
		}
		return s
	default:
		return v
	}
}


