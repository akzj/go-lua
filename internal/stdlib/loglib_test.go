package stdlib

import (
	"bytes"
	"strings"
	"testing"
)

func TestLogInfo(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.info("hello", 42)
	`)

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", output)
	}
	if !strings.Contains(output, "42") {
		t.Errorf("expected '42' in output, got: %s", output)
	}
	// Should contain line number (line 3 of the chunk)
	if !strings.Contains(output, ":3]") {
		t.Errorf("expected ':3]' (line number) in output, got: %s", output)
	}
}

func TestLogInfoTable(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.info({name = "Alice", hp = 100})
	`)

	output := buf.String()
	if !strings.Contains(output, "name = Alice") {
		t.Errorf("expected 'name = Alice' in output, got: %s", output)
	}
	if !strings.Contains(output, "hp = 100") {
		t.Errorf("expected 'hp = 100' in output, got: %s", output)
	}
}

func TestLogInfoArray(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.info({10, 20, 30})
	`)

	output := buf.String()
	if !strings.Contains(output, "[1] = 10") {
		t.Errorf("expected '[1] = 10' in output, got: %s", output)
	}
	if !strings.Contains(output, "[2] = 20") {
		t.Errorf("expected '[2] = 20' in output, got: %s", output)
	}
}

func TestLogWarnErrorDebug(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.warn("low hp")
		log.error("crash!")
		log.debug("trace")
	`)

	output := buf.String()
	if !strings.Contains(output, "[WARN") {
		t.Errorf("expected WARN prefix, got: %s", output)
	}
	if !strings.Contains(output, "low hp") {
		t.Errorf("expected 'low hp' in output, got: %s", output)
	}
	if !strings.Contains(output, "[ERROR") {
		t.Errorf("expected ERROR prefix, got: %s", output)
	}
	if !strings.Contains(output, "crash!") {
		t.Errorf("expected 'crash!' in output, got: %s", output)
	}
	if !strings.Contains(output, "[DEBUG") {
		t.Errorf("expected DEBUG prefix, got: %s", output)
	}
	if !strings.Contains(output, "trace") {
		t.Errorf("expected 'trace' in output, got: %s", output)
	}
}

func TestLogTime(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.time("test")
		local x = 0
		for i = 1, 1000 do x = x + i end
		log.time_end("test")
	`)

	output := buf.String()
	if !strings.Contains(output, "test:") {
		t.Errorf("expected timer label 'test:' in output, got: %s", output)
	}
}

func TestLogTimeDefault(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.time()
		log.time_end()
	`)

	output := buf.String()
	if !strings.Contains(output, "default:") {
		t.Errorf("expected 'default:' timer label in output, got: %s", output)
	}
}

func TestLogTimeNotFound(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.time_end("nonexistent")
	`)

	output := buf.String()
	if !strings.Contains(output, "timer not found") {
		t.Errorf("expected 'timer not found' in output, got: %s", output)
	}
}

func TestLogInfoNested(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.info({a = {1, 2, 3}})
	`)

	output := buf.String()
	// Nested table should show {...} instead of expanding
	if !strings.Contains(output, "a = {...}") {
		t.Errorf("expected nested table as '{...}' in output, got: %s", output)
	}
}

func TestLogInfoMixedTypes(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.info(nil, true, false, 3.14, "str")
	`)

	output := buf.String()
	if !strings.Contains(output, "nil") {
		t.Errorf("expected 'nil' in output, got: %s", output)
	}
	if !strings.Contains(output, "true") {
		t.Errorf("expected 'true' in output, got: %s", output)
	}
	if !strings.Contains(output, "false") {
		t.Errorf("expected 'false' in output, got: %s", output)
	}
	if !strings.Contains(output, "3.14") {
		t.Errorf("expected '3.14' in output, got: %s", output)
	}
	if !strings.Contains(output, "str") {
		t.Errorf("expected 'str' in output, got: %s", output)
	}
}

func TestLogInfoNoArgs(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	doString(t, L, `
		local log = require("log")
		log.info()
	`)

	output := buf.String()
	// Should still print location prefix
	if !strings.Contains(output, "[") {
		t.Errorf("expected location prefix in output, got: %s", output)
	}
}

func TestLogSetDepth(t *testing.T) {
	L := newState(t)
	defer L.Close()

	var buf bytes.Buffer
	L.Writer = &buf

	// Default depth=1: nested tables show {...}
	doString(t, L, `
		local log = require("log")
		log.info({a = {b = {c = 1}}})
	`)
	out1 := buf.String()
	if !strings.Contains(out1, "{...}") {
		t.Errorf("default depth should show {...}, got: %s", out1)
	}

	// depth=3: should expand 3 levels deep
	buf.Reset()
	doString(t, L, `
		local log = require("log")
		log.set_depth(3)
		log.info({a = {b = {c = 1}}})
	`)
	out2 := buf.String()
	if !strings.Contains(out2, "c = 1") {
		t.Errorf("depth=3 should expand 3 levels, got: %s", out2)
	}

	// depth=0: all tables show as {...}
	buf.Reset()
	doString(t, L, `
		local log = require("log")
		log.set_depth(0)
		log.info({a = 1})
	`)
	out3 := buf.String()
	if !strings.Contains(out3, "{...}") {
		t.Errorf("depth=0 should show {...}, got: %s", out3)
	}
}
