package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// ParsedPackage holds the parsed AST for a Go package.
type ParsedPackage struct {
	Name  string
	Fset  *token.FileSet
	Files []*ast.File
}

// parsePackage parses a Go package directory and returns the AST.
func parsePackage(path string) (*ParsedPackage, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing package: %w", err)
	}

	// Take the first non-test package.
	for name, pkg := range pkgs {
		if strings.HasSuffix(name, "_test") {
			continue
		}
		var files []*ast.File
		for _, f := range pkg.Files {
			files = append(files, f)
		}
		return &ParsedPackage{Name: name, Fset: fset, Files: files}, nil
	}
	return nil, fmt.Errorf("no Go package found in %s", absPath)
}
