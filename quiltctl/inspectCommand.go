package main

import (
	"flag"

	"github.com/NetSys/quilt/inspect"
)

type inspectCommand struct {
	flags *flag.FlagSet
}

func (iCmd *inspectCommand) getFlagSet() *flag.FlagSet {
	iCmd.flags = flag.NewFlagSet("inspect", flag.ExitOnError)

	return iCmd.flags
}

func (iCmd *inspectCommand) run() {
	// XXX: move the parsing code here?
	inspect.Main(iCmd.flags.Args())
}
