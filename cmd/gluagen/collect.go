package main

import (
	"go/ast"
	"go/token"
	"reflect"
	"strings"
)

// Bindings holds all collected binding information for code generation.
type Bindings struct {
	PkgName     string     // Go package name
	ModuleName  string     // Lua module name
	ImportPath  string     // import path for scan mode (e.g., "strings", "path/filepath")
	ImportAlias string     // Go package alias (e.g., "filepath" for "path/filepath")
	Funcs       []FuncInfo // standalone functions
	Types       []TypeInfo // struct types
}

// FuncInfo describes a function to bind.
type FuncInfo struct {
	GoName   string       // Go function name
	LuaName  string       // Lua name (snake_case by default)
	Receiver string       // receiver type name (empty for standalone)
	IsPtr    bool         // receiver is pointer
	Params   []ParamInfo  // parameters
	Returns  []ReturnInfo // return values
	HasError bool         // last return is error
}

// TypeInfo describes a struct type to bind.
type TypeInfo struct {
	GoName  string      // Go type name
	LuaName string      // Lua metatable name
	Fields  []FieldInfo // exported fields
	Methods []FuncInfo  // methods
}

// ParamInfo describes a function parameter.
type ParamInfo struct {
	Name           string
	GoType         string // "string", "int", "os.FileMode", "*Player", etc.
	UnderlyingKind string // underlying primitive: "uint32", "string", etc. (scan mode only)
	IsPtr          bool
	IsSlice        bool
	IsMap          bool
}

// ReturnInfo describes a return value.
type ReturnInfo struct {
	GoType         string
	UnderlyingKind string // underlying primitive (scan mode only)
	IsPtr          bool
	IsError        bool
}

// FieldInfo describes a struct field.
type FieldInfo struct {
	GoName   string // Go field name
	LuaName  string // Lua field name (from `lua:"name"` tag or snake_case)
	GoType   string // Go type
	ReadOnly bool   // `lua:",readonly"` tag
}

// addMethod adds a method to the matching type, or creates a placeholder type.
func (b *Bindings) addMethod(fi FuncInfo) {
	for i := range b.Types {
		if b.Types[i].GoName == fi.Receiver {
			b.Types[i].Methods = append(b.Types[i].Methods, fi)
			return
		}
	}
	// Type not yet seen — create a placeholder that will be merged later.
	b.Types = append(b.Types, TypeInfo{
		GoName:  fi.Receiver,
		LuaName: toSnakeCase(fi.Receiver),
		Methods: []FuncInfo{fi},
	})
}

// collectBindings scans the parsed package for //glua:bind annotations.
func collectBindings(pkg *ParsedPackage) *Bindings {
	b := &Bindings{
		PkgName:    pkg.Name,
		ModuleName: toSnakeCase(pkg.Name),
	}

	for _, file := range pkg.Files {
		// Check for file-level //glua:module annotation in the file doc comment.
		if file.Doc != nil {
			for _, c := range file.Doc.List {
				if mod := parseModuleDirective(c.Text); mod != "" {
					b.ModuleName = mod
				}
			}
		}

		// Also scan all comments for //glua:module (it may appear outside file doc).
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if mod := parseModuleDirective(c.Text); mod != "" {
					b.ModuleName = mod
				}
			}
		}

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if hasGluaBind(d.Doc) {
					fi := extractFunc(d)
					fi.LuaName = parseLuaName(d.Doc, fi.GoName)
					if fi.Receiver != "" {
						b.addMethod(fi)
					} else {
						b.Funcs = append(b.Funcs, fi)
					}
				}
			case *ast.GenDecl:
				if d.Tok == token.TYPE && hasGluaBind(d.Doc) {
					for _, spec := range d.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						st, ok := ts.Type.(*ast.StructType)
						if !ok {
							continue
						}
						ti := extractType(ts, st)
						ti.LuaName = parseLuaName(d.Doc, ti.GoName)
						// Merge with existing placeholder if methods were seen first.
						merged := false
						for i := range b.Types {
							if b.Types[i].GoName == ti.GoName {
								b.Types[i].Fields = ti.Fields
								if ti.LuaName != "" {
									b.Types[i].LuaName = ti.LuaName
								}
								merged = true
								break
							}
						}
						if !merged {
							b.Types = append(b.Types, ti)
						}
					}
				}
			}
		}
	}

	return b
}

// hasGluaBind checks if a comment group contains //glua:bind.
func hasGluaBind(cg *ast.CommentGroup) bool {
	if cg == nil {
		return false
	}
	for _, c := range cg.List {
		if strings.HasPrefix(strings.TrimSpace(c.Text), "//glua:bind") {
			return true
		}
	}
	return false
}

// parseLuaName extracts name=xxx from //glua:bind name=xxx.
func parseLuaName(cg *ast.CommentGroup, defaultName string) string {
	if cg == nil {
		return toSnakeCase(defaultName)
	}
	for _, c := range cg.List {
		text := strings.TrimSpace(c.Text)
		if strings.HasPrefix(text, "//glua:bind") {
			rest := strings.TrimPrefix(text, "//glua:bind")
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "name=") {
				return strings.TrimPrefix(rest, "name=")
			}
		}
	}
	return toSnakeCase(defaultName)
}

// parseModuleDirective extracts module name from //glua:module name=xxx.
func parseModuleDirective(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "//glua:module") {
		return ""
	}
	rest := strings.TrimPrefix(text, "//glua:module")
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "name=") {
		return strings.TrimPrefix(rest, "name=")
	}
	return ""
}

// extractFunc extracts FuncInfo from an ast.FuncDecl.
func extractFunc(fd *ast.FuncDecl) FuncInfo {
	fi := FuncInfo{
		GoName: fd.Name.Name,
	}

	// Receiver
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		recv := fd.Recv.List[0]
		switch t := recv.Type.(type) {
		case *ast.StarExpr:
			fi.Receiver = exprToString(t.X)
			fi.IsPtr = true
		case *ast.Ident:
			fi.Receiver = t.Name
		}
	}

	// Parameters
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			pi := paramFromExpr(field.Type)
			if len(field.Names) == 0 {
				pi.Name = "_"
				fi.Params = append(fi.Params, pi)
			} else {
				for _, name := range field.Names {
					p := pi // copy
					p.Name = name.Name
					fi.Params = append(fi.Params, p)
				}
			}
		}
	}

	// Returns
	if fd.Type.Results != nil {
		for _, field := range fd.Type.Results.List {
			ri := returnFromExpr(field.Type)
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for range count {
				fi.Returns = append(fi.Returns, ri)
			}
		}
		// Check if last return is error
		if len(fi.Returns) > 0 && fi.Returns[len(fi.Returns)-1].GoType == "error" {
			fi.HasError = true
			fi.Returns[len(fi.Returns)-1].IsError = true
		}
	}

	return fi
}

// extractType extracts TypeInfo from an ast.TypeSpec + ast.StructType.
func extractType(ts *ast.TypeSpec, st *ast.StructType) TypeInfo {
	ti := TypeInfo{
		GoName:  ts.Name.Name,
		LuaName: toSnakeCase(ts.Name.Name),
	}

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue // embedded field — skip
		}
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			fi := FieldInfo{
				GoName: name.Name,
				GoType: exprToString(field.Type),
			}
			// Parse struct tag
			fi.LuaName, fi.ReadOnly = parseLuaTag(field.Tag, name.Name)
			if fi.LuaName == "-" {
				continue // skip fields tagged lua:"-"
			}
			ti.Fields = append(ti.Fields, fi)
		}
	}

	return ti
}

// parseLuaTag parses the `lua:"name,readonly"` struct tag.
func parseLuaTag(tag *ast.BasicLit, defaultName string) (string, bool) {
	if tag == nil {
		return toSnakeCase(defaultName), false
	}
	// tag.Value includes backticks: `lua:"name"`
	tagStr := tag.Value
	if len(tagStr) >= 2 {
		tagStr = tagStr[1 : len(tagStr)-1] // strip backticks
	}
	luaTag := reflect.StructTag(tagStr).Get("lua")
	if luaTag == "" {
		return toSnakeCase(defaultName), false
	}
	parts := strings.Split(luaTag, ",")
	name := parts[0]
	if name == "" {
		name = toSnakeCase(defaultName)
	}
	readOnly := false
	for _, p := range parts[1:] {
		if strings.TrimSpace(p) == "readonly" {
			readOnly = true
		}
	}
	return name, readOnly
}

// exprToString converts an AST type expression to a string representation.
func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + exprToString(t.Elt)
		}
		return "[...]" + exprToString(t.Elt)
	case *ast.MapType:
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "any"
	}
}

// paramFromExpr creates a ParamInfo from a type expression.
func paramFromExpr(expr ast.Expr) ParamInfo {
	goType := exprToString(expr)
	pi := ParamInfo{GoType: goType}
	switch expr.(type) {
	case *ast.StarExpr:
		pi.IsPtr = true
	case *ast.ArrayType:
		pi.IsSlice = true
	case *ast.MapType:
		pi.IsMap = true
	}
	return pi
}

// returnFromExpr creates a ReturnInfo from a type expression.
func returnFromExpr(expr ast.Expr) ReturnInfo {
	goType := exprToString(expr)
	ri := ReturnInfo{GoType: goType}
	if _, ok := expr.(*ast.StarExpr); ok {
		ri.IsPtr = true
	}
	if goType == "error" {
		ri.IsError = true
	}
	return ri
}
