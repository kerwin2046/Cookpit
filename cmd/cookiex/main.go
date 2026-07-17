package main

import (
	"fmt"
	"io"
	"os"

	"cookiex/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	services, err := cli.DefaultServices()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	command := cli.NewRootCommand(services)
	command.SetArgs(args)
	command.SetOut(stdout)
	command.SetErr(stderr)
	if err := command.Execute(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
