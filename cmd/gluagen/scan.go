package main

import (
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"strings"
)

// scanPackage loads a Go package by import path using go/types and collects
// all bindable exported functions (those with supported parameter/return types).
func scanPackage(importPath, includeStr, excludeStr string) (*Bindings, error) {
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)

	pkg, err := imp.Import(importPath)
	if err != nil {
		return nil, fmt.Errorf("cannot import %q: %w", importPath, err)
	}

	// Parse include/exclude filter lists.
	includeSet := parseCSV(includeStr)
	excludeSet := parseCSV(excludeStr)

	b := &Bindings{
		PkgName:     "bindings", // default; overridden by -pkg flag
		ModuleName:  pkg.Name(),
		ImportPath:  importPath,
		ImportAlias: pkg.Name(),
	}

	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}

		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}

		sig := fn.Type().(*types.Signature)

		// Skip methods — only package-level functions in scan mode.
		if sig.Recv() != nil {
			continue
		}

		// Apply include/exclude filters.
		if len(includeSet) > 0 && !includeSet[fn.Name()] {
			continue
		}
		if excludeSet[fn.Name()] {
			continue
		}

		// Only bind functions whose params and returns are all supported types.
		if !isSignatureBindable(sig) {
			continue
		}

		fi := sigToFuncInfo(fn.Name(), sig)
		b.Funcs = append(b.Funcs, fi)
	}

	if len(b.Funcs) == 0 {
		return nil, fmt.Errorf("no bindable functions found in %q", importPath)
	}

	return b, nil
}

// isSignatureBindable returns true if every param and result has a supported type.
func isSignatureBindable(sig *types.Signature) bool {
	if sig.Variadic() {
		return false // skip variadic functions
	}
	for i := range sig.Params().Len() {
		if !isSupportedType(sig.Params().At(i).Type()) {
			return false
		}
	}
	for i := range sig.Results().Len() {
		if !isSupportedType(sig.Results().At(i).Type()) {
			return false
		}
	}
	return true
}

// isSupportedType checks if a Go type can be automatically converted to/from Lua.
func isSupportedType(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.String, types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64:
			return true
		}
	case *types.Slice:
		// []byte and []string are supported.
		if basic, ok := u.Elem().Underlying().(*types.Basic); ok {
			return basic.Kind() == types.Byte || basic.Kind() == types.String
		}
	case *types.Interface:
		// error interface is supported.
		if isErrorType(t) {
			return true
		}
	}
	return false
}

// isErrorType checks if a type is the built-in error interface.
func isErrorType(t types.Type) bool {
	// The canonical check: does it implement the error interface?
	if iface, ok := t.Underlying().(*types.Interface); ok {
		if iface.NumMethods() == 1 && iface.Method(0).Name() == "Error" {
			return true
		}
	}
	return false
}

// sigToFuncInfo converts a types.Signature to our FuncInfo struct.
func sigToFuncInfo(name string, sig *types.Signature) FuncInfo {
	fi := FuncInfo{
		GoName:  name,
		LuaName: toSnakeCase(name),
	}

	params := sig.Params()
	for i := range params.Len() {
		p := params.At(i)
		goType := typeToGoString(p.Type())
		kind := underlyingKindString(p.Type())
		pi := ParamInfo{
			Name:           p.Name(),
			GoType:         goType,
			UnderlyingKind: kind,
			IsSlice:        strings.HasPrefix(goType, "[]"),
		}
		fi.Params = append(fi.Params, pi)
	}

	results := sig.Results()
	for i := range results.Len() {
		r := results.At(i)
		goType := typeToGoString(r.Type())
		kind := underlyingKindString(r.Type())
		ri := ReturnInfo{
			GoType:         goType,
			UnderlyingKind: kind,
			IsError:        goType == "error",
		}
		fi.Returns = append(fi.Returns, ri)
	}

	if len(fi.Returns) > 0 && fi.Returns[len(fi.Returns)-1].IsError {
		fi.HasError = true
	}

	return fi
}

// typeToGoString converts a types.Type to the Go type string used in codegen.
// Named/aliased types from external packages are preserved (e.g., "os.FileMode").
func typeToGoString(t types.Type) string {
	// Check for type aliases first (e.g., os.FileMode = io/fs.FileMode).
	if alias, ok := t.(*types.Alias); ok {
		obj := alias.Obj()
		pkg := obj.Pkg()
		if pkg != nil {
			return pkg.Name() + "." + obj.Name()
		}
		return obj.Name()
	}

	// Check for named types (e.g., io/fs.FileMode).
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		pkg := obj.Pkg()
		if pkg != nil {
			// External named type: qualified name (e.g., "fs.FileMode").
			return pkg.Name() + "." + obj.Name()
		}
		// Built-in named type (e.g., "error").
		return obj.Name()
	}

	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Byte: // uint8 alias
			return "uint8"
		case types.Rune: // int32 alias
			return "int32"
		default:
			return u.Name()
		}
	case *types.Slice:
		return "[]" + typeToGoString(u.Elem())
	case *types.Interface:
		if isErrorType(t) {
			return "error"
		}
		return "any"
	}
	return t.String()
}

// underlyingKindString returns the underlying primitive type name for a type.
// For named types like os.FileMode (underlying uint32), returns "uint32".
// For basic types, returns the basic name. For slices, returns "[]byte" or "[]string".
func underlyingKindString(t types.Type) string {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Byte:
			return "uint8"
		case types.Rune:
			return "int32"
		default:
			return u.Name()
		}
	case *types.Slice:
		if basic, ok := u.Elem().Underlying().(*types.Basic); ok {
			if basic.Kind() == types.Byte {
				return "[]byte"
			}
			if basic.Kind() == types.String {
				return "[]string"
			}
		}
	case *types.Interface:
		if isErrorType(t) {
			return "error"
		}
	}
	return ""
}

// parseCSV splits a comma-separated string into a set.
func parseCSV(s string) map[string]bool {
	if s == "" {
		return nil
	}
	m := make(map[string]bool)
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			m[item] = true
		}
	}
	return m
}
