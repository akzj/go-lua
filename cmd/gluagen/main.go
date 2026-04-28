// Command gluagen generates Lua binding code for Go packages.
//
// Annotation mode (default):
//
//	gluagen [flags] <package-path>
//
// Scan mode (for existing packages):
//
//	gluagen --scan <import-path> [flags]
//
// Flags:
//
//	-o file        Output file (default: stdout)
//	-pkg name      Output package name (default: same as input; "bindings" in scan mode)
//	-module name   Lua module name (default: package name)
//	-scan path     Scan a Go package by import path (e.g., "strings", "math")
//	-include list  Comma-separated functions to include (scan mode only)
//	-exclude list  Comma-separated functions to exclude (scan mode only)
//
// In annotation mode, gluagen scans the specified Go package directory for
// //glua:bind annotations and generates type-safe Lua binding code.
//
// In scan mode, gluagen uses go/types to load any Go package by import path
// and generates bindings for all exported functions with supported parameter types.
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
	scanPkg := flag.String("scan", "", "scan a Go package by import path (e.g., \"strings\", \"math\")")
	include := flag.String("include", "", "comma-separated list of functions to include (scan mode)")
	exclude := flag.String("exclude", "", "comma-separated list of functions to exclude (scan mode)")
	flag.Parse()

	var code string

	if *scanPkg != "" {
		// Scan mode: use go/types to load package and generate bindings.
		bindings, err := scanPackage(*scanPkg, *include, *exclude)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gluagen: %v\n", err)
			os.Exit(1)
		}
		if *moduleName != "" {
			bindings.ModuleName = *moduleName
		}
		if *pkgName != "" {
			bindings.PkgName = *pkgName
		}
		code = generateScan(bindings)
	} else {
		// Annotation mode: parse source directory for //glua:bind annotations.
		if flag.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: gluagen [flags] <package-path>")
			fmt.Fprintln(os.Stderr, "       gluagen --scan <import-path> [flags]")
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
		code = generate(bindings)
	}

	// Write output
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(code), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "gluagen: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(code)
	}
}
