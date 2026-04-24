package lua_test

import (
	"testing"
	"testing/fstest"

	"github.com/akzj/go-lua/pkg/lua"
)

func TestSetFileSystem_DoFile(t *testing.T) {
	testFS := fstest.MapFS{
		"test.lua": &fstest.MapFile{Data: []byte(`return 42`)},
	}

	L := lua.NewState()
	defer L.Close()
	L.SetFileSystem(testFS)

	err := L.DoFile("test.lua")
	if err != nil {
		t.Fatalf("DoFile from FS failed: %v", err)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 42 {
		t.Fatalf("expected 42, got %d (ok=%v)", n, ok)
	}
}

func TestSetFileSystem_LoadFile(t *testing.T) {
	testFS := fstest.MapFS{
		"chunk.lua": &fstest.MapFile{Data: []byte(`return "hello"`)},
	}

	L := lua.NewState()
	defer L.Close()
	L.SetFileSystem(testFS)

	status := L.LoadFile("chunk.lua", "t")
	if status != lua.OK {
		msg, _ := L.ToString(-1)
		t.Fatalf("LoadFile failed: %s", msg)
	}
	// Execute the loaded chunk
	err := L.CallSafe(0, 1)
	if err != nil {
		t.Fatalf("CallSafe failed: %v", err)
	}
	s, _ := L.ToString(-1)
	if s != "hello" {
		t.Fatalf("expected 'hello', got %q", s)
	}
}

func TestSetFileSystem_Require(t *testing.T) {
	testFS := fstest.MapFS{
		"mymod.lua": &fstest.MapFile{Data: []byte(`return {name="mymod", value=99}`)},
	}

	L := lua.NewState()
	defer L.Close()
	L.SetFileSystem(testFS)

	// Set package.path to use our FS (simple template)
	err := L.DoString(`package.path = "?.lua"`)
	if err != nil {
		t.Fatalf("failed to set package.path: %v", err)
	}

	err = L.DoString(`
		local m = require("mymod")
		assert(m.name == "mymod", "expected name='mymod', got " .. tostring(m.name))
		assert(m.value == 99, "expected value=99, got " .. tostring(m.value))
	`)
	if err != nil {
		t.Fatalf("require from FS failed: %v", err)
	}
}

func TestSetFileSystem_RequireSubdir(t *testing.T) {
	testFS := fstest.MapFS{
		"lib/utils.lua": &fstest.MapFile{Data: []byte(`return {helper=true}`)},
	}

	L := lua.NewState()
	defer L.Close()
	L.SetFileSystem(testFS)

	err := L.DoString(`package.path = "?.lua;lib/?.lua"`)
	if err != nil {
		t.Fatalf("failed to set package.path: %v", err)
	}

	err = L.DoString(`
		local u = require("utils")
		assert(u.helper == true)
	`)
	if err != nil {
		t.Fatalf("require from FS subdir failed: %v", err)
	}
}

func TestSetFileSystem_Nil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// SetFileSystem(nil) should not panic
	L.SetFileSystem(nil)

	// FileSystem() should return nil for default
	if L.FileSystem() != nil {
		t.Fatal("expected nil FileSystem for default")
	}

	// Normal execution should work
	err := L.DoString(`return 1`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetFileSystem_FileNotFound(t *testing.T) {
	testFS := fstest.MapFS{
		"exists.lua": &fstest.MapFile{Data: []byte(`return 1`)},
	}

	L := lua.NewState()
	defer L.Close()
	L.SetFileSystem(testFS)

	err := L.DoFile("notexists.lua")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	t.Logf("Got expected error: %v", err)
}

func TestSetFileSystem_DoFileWithShebang(t *testing.T) {
	testFS := fstest.MapFS{
		"script.lua": &fstest.MapFile{Data: []byte("#!/usr/bin/env lua\nreturn 7")},
	}

	L := lua.NewState()
	defer L.Close()
	L.SetFileSystem(testFS)

	err := L.DoFile("script.lua")
	if err != nil {
		t.Fatalf("DoFile with shebang failed: %v", err)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 7 {
		t.Fatalf("expected 7, got %d (ok=%v)", n, ok)
	}
}

func TestSetFileSystem_RequireDotNotation(t *testing.T) {
	// Test that "foo.bar" maps to "foo/bar.lua" in the FS
	testFS := fstest.MapFS{
		"foo/bar.lua": &fstest.MapFile{Data: []byte(`return "foo.bar loaded"`)},
	}

	L := lua.NewState()
	defer L.Close()
	L.SetFileSystem(testFS)

	err := L.DoString(`package.path = "?.lua"`)
	if err != nil {
		t.Fatalf("failed to set package.path: %v", err)
	}

	err = L.DoString(`
		local result = require("foo.bar")
		assert(result == "foo.bar loaded", "got: " .. tostring(result))
	`)
	if err != nil {
		t.Fatalf("require with dot notation failed: %v", err)
	}
}
