package cmd

import (
	"fmt"
	"io"
	"os"
)

// Execute parses command line arguments for the easygin CLI.
func Execute(args []string) error {
	return execute(args, os.Stdout)
}

func execute(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stdout, usage())
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, usage())
		return nil
	case "new":
		return runNew(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage())
	}
}

func usage() string {
	return `easygin CLI

Usage:
  easygin new <project> [--module <module>] [--service-name <name>] [--router-path <path>]
  easygin help
`
}
