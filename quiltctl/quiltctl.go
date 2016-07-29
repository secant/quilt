package main

import (
	"flag"
	"fmt"
	"os"
)

type subcommand interface {
	// The function to run once the flags have been parsed.
	run()

	// Create a flag set that, when parsed, will populate fields for `run()`.
	getFlagSet() *flag.FlagSet
}

var commands = map[string]subcommand{
	"queryMachines":   &machineCommand{},
	"queryContainers": &containerCommand{},
	"run":             &runCommand{},
	"stop":            &stopCommand{},
}

func main() {
	flag.Usage = func() {
		flag.PrintDefaults()
	}

	subCommandName := os.Args[1]
	if subCommand, ok := commands[subCommandName]; ok {
		subCommand.getFlagSet().Parse(os.Args[2:])
		// XXX: Check for parse error?
		subCommand.run()
	} else {
		fmt.Printf("Unrecongized subcommand: %s", subCommandName)
	}
}
