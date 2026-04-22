// Command luarunner is a minimal go-lua CLI for running Lua scripts.
// It is used by tools/luabench.sh to benchmark go-lua against C Lua.
//
// For a full-featured interpreter with REPL support, see cmd/glua/.
package main

import (
	"fmt"
	"os"

	"github.com/akzj/go-lua/pkg/lua"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: luarunner <file.lua> | -e <code>")
		os.Exit(1)
	}

	L := lua.NewState()

	var err error
	if os.Args[1] == "-e" {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: luarunner -e <code>")
			os.Exit(1)
		}
		err = L.DoString(os.Args[2])
	} else {
		err = L.DoFile(os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		L.Close()
		os.Exit(1)
	}
	L.Close()
}
