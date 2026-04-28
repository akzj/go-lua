// Command glua is a Lua 5.5 interpreter powered by go-lua.
//
// Usage:
//
//	glua [options] [script [args...]]
//
// Options:
//
//	-e "code"     Execute the given Lua code
//	-l name       Require library 'name' before executing scripts
//	-i            Enter interactive REPL mode after executing scripts
//	-v            Print version information and exit
//	-             Read script from standard input
//	--sandbox     Run in sandbox mode (CPU limited)
//	--timeout N   Set execution timeout in seconds
//	--no-stdlib   Don't load standard libraries
//
// When invoked with no arguments, glua enters interactive REPL mode.
// Multiple -e and -l flags can be combined; they execute in order.
//
// The REPL supports expression evaluation: typing an expression like "1+2"
// automatically prints the result (wraps in "return <expr>" internally).
// Multi-line input is supported — if a statement is incomplete, the prompt
// changes to ">> " and waits for more input.
//
// Shebang support: scripts can start with #!/usr/bin/env glua and be
// executed directly as programs on Unix systems.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/akzj/go-lua/pkg/lua"
)

const version = "glua 0.8.2 (go-lua, Lua 5.5)"

func main() {
	os.Exit(run())
}

// action represents a pre-script action (-e snippet or -l require).
type action struct {
	kind string // "exec" or "lib"
	arg  string // code snippet or library name
}

func run() int {
	args := os.Args[1:]

	// Parse flags manually (to support Lua-style argument ordering).
	var (
		actions        []action // -e and -l actions in order
		files          []string // script files
		interactive    bool     // -i flag
		showVersion    bool     // -v flag
		readStdin      bool     // - flag
		sandboxMode    bool     // --sandbox flag
		bareMode       bool     // --no-stdlib flag
		timeoutSeconds int      // --timeout N
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-v":
			showVersion = true
		case "-i":
			interactive = true
		case "-e":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "glua: '-e' needs argument")
				return 1
			}
			actions = append(actions, action{kind: "exec", arg: args[i]})
		case "-l":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "glua: '-l' needs argument")
				return 1
			}
			actions = append(actions, action{kind: "lib", arg: args[i]})
		case "-":
			readStdin = true
		case "--sandbox":
			sandboxMode = true
		case "--no-stdlib":
			bareMode = true
		case "--timeout":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "glua: '--timeout' needs argument (seconds)")
				return 1
			}
			sec, parseErr := strconv.Atoi(args[i])
			if parseErr != nil {
				fmt.Fprintf(os.Stderr, "glua: invalid timeout: %s\n", args[i])
				return 1
			}
			timeoutSeconds = sec
		case "--":
			// Everything after -- is script arguments, not glua flags.
			files = append(files, args[i+1:]...)
			i = len(args) // stop parsing
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "glua: unrecognized option '%s'\n", args[i])
				printUsage()
				return 1
			}
			// First non-flag argument is the script; rest are script args.
			files = append(files, args[i])
			// Remaining args are passed to the Lua script via 'arg' table.
			break
		}
	}

	if showVersion {
		fmt.Println(version)
		return 0
	}

	// If no actions specified, default to interactive mode.
	if len(actions) == 0 && len(files) == 0 && !readStdin {
		interactive = true
	}

	// Create the Lua state based on mode flags.
	var L *lua.State
	switch {
	case sandboxMode:
		L = lua.NewSandboxState(lua.SandboxConfig{
			CPULimit: 10_000_000, // 10M instructions default
		})
	case bareMode:
		L = lua.NewBareState()
	default:
		L = lua.NewState()
	}
	defer L.Close()

	// Apply execution timeout if requested.
	if timeoutSeconds > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
		L.SetContext(ctx)
	}

	// Set up the global 'arg' table (like C Lua).
	setupArgTable(L, files)

	// Execute -e and -l actions in order.
	for _, a := range actions {
		switch a.kind {
		case "exec":
			if err := L.DoString(a.arg); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		case "lib":
			// require("name") — equivalent to C Lua's -l flag.
			code := fmt.Sprintf("require(%q)", a.arg)
			if err := L.DoString(code); err != nil {
				fmt.Fprintf(os.Stderr, "glua: error loading library '%s': %v\n", a.arg, err)
				return 1
			}
		}
	}

	// Execute script files.
	for _, file := range files {
		if err := L.DoFile(file); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	// Read from stdin if - was specified.
	if readStdin {
		code, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "glua: error reading stdin: %v\n", err)
			return 1
		}
		if err := L.DoString(string(code)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	// Interactive REPL.
	if interactive {
		return repl(L)
	}

	return 0
}

// setupArgTable creates the global 'arg' table with script arguments.
func setupArgTable(L *lua.State, files []string) {
	L.NewTable()
	L.PushString("glua")
	L.SetI(-2, 0)
	for i, f := range files {
		L.PushString(f)
		L.SetI(-2, int64(i+1))
	}
	L.SetGlobal("arg")
}

// repl runs an interactive read-eval-print loop.
func repl(L *lua.State) int {
	fmt.Println(version)
	fmt.Println("Type Lua code to execute. Press Ctrl+D to exit.")

	scanner := bufio.NewScanner(os.Stdin)
	var buf strings.Builder

	prompt := "> "
	for {
		fmt.Print(prompt)
		if !scanner.Scan() {
			break // EOF
		}
		line := scanner.Text()

		if buf.Len() == 0 {
			// Try as an expression first: wrap in "return <expr>"
			// so the user can type "1+2" and see the result.
			if tryExpr(L, line) {
				continue
			}
		}

		buf.WriteString(line)
		buf.WriteByte('\n')

		code := buf.String()

		// Try to load the accumulated code.
		status := L.Load(code, "=stdin", "t")
		if status == lua.ErrSyntax {
			// Check if the error is an incomplete statement
			// (i.e., the parser wants more input).
			msg, _ := L.ToString(-1)
			L.Pop(1)
			if isIncomplete(msg) {
				prompt = ">> "
				continue
			}
			// Real syntax error — report it.
			fmt.Fprintln(os.Stderr, msg)
			buf.Reset()
			prompt = "> "
			continue
		}
		if status != lua.OK {
			msg, _ := L.ToString(-1)
			fmt.Fprintln(os.Stderr, msg)
			L.Pop(1)
			buf.Reset()
			prompt = "> "
			continue
		}

		// Execute the loaded chunk.
		callStatus := L.PCall(0, lua.MultiRet, 0)
		if callStatus != lua.OK {
			msg, _ := L.ToString(-1)
			fmt.Fprintln(os.Stderr, msg)
			L.Pop(1)
		} else {
			// Print any return values.
			printResults(L)
		}

		buf.Reset()
		prompt = "> "
	}

	fmt.Println() // newline after EOF
	return 0
}

// tryExpr tries to evaluate a line as "return <expr>".
// If it succeeds, prints the result(s) and returns true.
func tryExpr(L *lua.State, line string) bool {
	code := "return " + line
	status := L.Load(code, "=stdin", "t")
	if status != lua.OK {
		L.Pop(1) // pop error message
		return false
	}

	callStatus := L.PCall(0, lua.MultiRet, 0)
	if callStatus != lua.OK {
		L.Pop(1) // pop error
		return false
	}

	printResults(L)
	return true
}

// printResults prints all values on the stack (above the base) using tostring.
func printResults(L *lua.State) {
	n := L.GetTop()
	if n == 0 {
		return
	}
	for i := 1; i <= n; i++ {
		if i > 1 {
			fmt.Print("\t")
		}
		s := L.TolString(i)
		fmt.Print(s)
		L.Pop(1) // pop the string pushed by TolString
	}
	fmt.Println()
	L.SetTop(0)
}

// isIncomplete returns true if the syntax error indicates an incomplete
// statement (e.g., missing 'end', unterminated string).
func isIncomplete(msg string) bool {
	// C Lua uses "<eof>" at the end of incomplete-statement errors.
	return strings.HasSuffix(msg, "<eof>")
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: glua [options] [script [args...]]")
	fmt.Fprintln(os.Stderr, "  -e code       execute string 'code'")
	fmt.Fprintln(os.Stderr, "  -l name       require library 'name'")
	fmt.Fprintln(os.Stderr, "  -i            enter interactive mode after executing script")
	fmt.Fprintln(os.Stderr, "  -v            show version information")
	fmt.Fprintln(os.Stderr, "  -             read from standard input")
	fmt.Fprintln(os.Stderr, "  --sandbox     run in sandbox mode (CPU limited)")
	fmt.Fprintln(os.Stderr, "  --timeout N   set execution timeout in seconds")
	fmt.Fprintln(os.Stderr, "  --no-stdlib   don't load standard libraries")
}
