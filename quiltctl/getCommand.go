package main

import (
	"flag"
	"fmt"

	"github.com/NetSys/quilt/stitch"
)

type getCommand struct {
	importPath string
}

func (gCmd *getCommand) getFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("get", flag.ExitOnError)

	flags.StringVar(&gCmd.importPath, "importPath", "", "the stitch to download")

	return flags
}

func (gCmd *getCommand) run() {
	if err := stitch.GetSpec(gCmd.importPath); err != nil {
		fmt.Println(err.Error())
	}
}
