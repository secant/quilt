package main

import (
	"flag"
	"fmt"

	"github.com/NetSys/quilt/api"
)

type machineCommand struct {
	host string
}

func (mCmd *machineCommand) getFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("queryMachines", flag.ExitOnError)

	flags.StringVar(&mCmd.host, "H", api.DefaultSocket, "the host to connect to")

	return flags
}

func (mCmd *machineCommand) run() {
	client, err := api.NewClient(mCmd.host)
	if err != nil {
		fmt.Printf(err.Error())
	}

	allMachines, err := client.QueryMachines()
	if err != nil {
		fmt.Println(err.Error())
	}

	fmt.Printf("Machines: %v\n", allMachines)
}
