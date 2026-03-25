package main

import (
	"fmt"
	"io"
	"os"

	"github.com/akzj/go-lua/pkg/api"
)

func main() {
	L := api.NewState()
	defer L.Close()

	L.OpenLibs()

	if len(os.Args) > 1 {
		// Execute file
		filename := os.Args[1]
		if err := L.DoFile(filename); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", filename, err)
			os.Exit(1)
		}
	} else {
		// Read from stdin
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