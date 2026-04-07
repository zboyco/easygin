package main

import (
	"fmt"
	"os"

	easygincmd "github.com/zboyco/easygin/cmd/easygin/cmd"
)

func main() {
	if err := easygincmd.Execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
