package main

import (
	"bufio"
	"flag"
	"fmt"
	"path/filepath"
	"text/scanner"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/stitch"
	"github.com/NetSys/quilt/util"
)

type runCommand struct {
	stitch string

	host string
}

func (rCmd *runCommand) getFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("run", flag.ExitOnError)

	flags.StringVar(&rCmd.stitch, "stitch", "", "the stitch to run")
	flags.StringVar(&rCmd.host, "host", api.DefaultSocket, "the host to connect to")

	return flags
}

func (rCmd *runCommand) run() {
	// XXX: Get the stitch from rCmd.flags if rCmd.host is empty.
	client, err := api.NewClient(rCmd.host)
	if err != nil {
		fmt.Printf(err.Error())
	}

	pathStr := stitch.GetQuiltPath()

	f, err := util.Open(rCmd.stitch)
	if err != nil {
		f, err = util.Open(filepath.Join(pathStr, rCmd.stitch))
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	defer f.Close()

	sc := scanner.Scanner{
		Position: scanner.Position{
			Filename: rCmd.stitch,
		},
	}

	spec, err := stitch.Compile(*sc.Init(bufio.NewReader(f)), pathStr, false)
	if err != nil {
		fmt.Println(err.Error())
	}

	err = client.RunStitch(spec)
	if err != nil {
		fmt.Println(err.Error())
	}
}
