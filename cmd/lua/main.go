package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/akzj/go-lua/pkg/api"
	"github.com/akzj/go-lua/pkg/lexer"
	"github.com/akzj/go-lua/pkg/parser"
)

func main() {
	L := api.NewState()
	defer L.Close()

	L.OpenLibs()

	if len(os.Args) > 1 {
		// Execute file
		filename := os.Args[1]
		if err := L.DoFile(filename); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	} else {
		// Check if stdin is a terminal (REPL mode) or piped
		if isTerminal() {
			// REPL mode
			runREPL(L)
		} else {
			// Read from stdin (piped mode)
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
				os.Exit(1)
			}
			if len(data) > 0 {
				if err := L.DoString(string(data), "stdin"); err != nil {
					fmt.Fprintf(os.Stderr, "stdin: %v\n", err)
					os.Exit(1)
				}
			}
		}
	}
}

// isTerminal returns true if stdin is a terminal (not piped)
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// runREPL starts an interactive REPL
func runREPL(L *api.State) {
	reader := bufio.NewReader(os.Stdin)
	
	fmt.Println("Lua 5.4 (go-lua)")
	fmt.Println("Type 'exit' or press Ctrl+D to quit")
	
	for {
		fmt.Print("> ")
		
		// Read line
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println()
				break
			}
			fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
			continue
		}
		
		// Trim newline
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		
		// Check for exit command
		if line == "exit" || line == "quit" {
			break
		}
		
		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}
		
		// Handle multi-line input for incomplete statements
		code := line
		for !isCompleteStatement(code) {
			fmt.Print(">> ")
			nextLine, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				continue
			}
			nextLine = strings.TrimSuffix(nextLine, "\n")
			nextLine = strings.TrimSuffix(nextLine, "\r")
			code += "\n" + nextLine
		}
		
		// Execute the code
		result := L.DoString(code, "<repl>")
		if result != nil {
			fmt.Fprintf(os.Stderr, "%v\n", result)
		}
	}
}

// isCompleteStatement checks if the code forms a complete Lua statement
func isCompleteStatement(code string) bool {
	// Try to parse the code
	l := lexer.NewLexer([]byte(code), "<repl>")
	p := parser.NewParser(l)
	
	// Try to parse - if it fails due to unexpected EOF, it's incomplete
	_, err := p.Parse()
	if err != nil {
		errStr := err.Error()
		// Check for common "incomplete" error patterns
		if strings.Contains(errStr, "unexpected EOF") ||
			strings.Contains(errStr, "unexpected $end") ||
			strings.Contains(errStr, "expected") {
			return false
		}
	}
	return true
}