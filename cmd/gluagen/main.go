// Command gluagen generates Lua binding code for Go packages.
//
// Usage:
//
//	gluagen [flags] <package-path>
//
// Flags:
//
//	-o file       Output file (default: stdout)
//	-pkg name     Output package name (default: same as input)
//	-module name  Lua module name (default: package name)
//
// gluagen scans the specified Go package for //glua:bind annotations
// and generates type-safe Lua binding code that uses the go-lua stack API
// directly (no reflection).
//
// Annotations:
//
//	//glua:bind                 — bind a function or type
//	//glua:bind name=lua_name   — custom Lua name
//	//glua:module name=modname  — set the Lua module name (file-level)
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	outputFile := flag.String("o", "", "output file (default: stdout)")
	pkgName := flag.String("pkg", "", "output package name (default: same as input)")
	moduleName := flag.String("module", "", "Lua module name (default: package name)")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: gluagen [flags] <package-path>")
		os.Exit(1)
	}

	pkgPath := flag.Arg(0)

	// 1. Parse the package
	pkg, err := parsePackage(pkgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gluagen: %v\n", err)
		os.Exit(1)
	}

	// 2. Collect bindings
	bindings := collectBindings(pkg)
	if len(bindings.Funcs) == 0 && len(bindings.Types) == 0 {
		fmt.Fprintln(os.Stderr, "gluagen: no //glua:bind annotations found")
		os.Exit(1)
	}

	// 3. Resolve names
	if *moduleName != "" {
		bindings.ModuleName = *moduleName
	}
	if *pkgName != "" {
		bindings.PkgName = *pkgName
	}

	// 4. Generate code
	code := generate(bindings)

	// 5. Write output
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(code), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "gluagen: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(code)
	}
}
