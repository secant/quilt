package main

import (
	"flag"
	"fmt"

	"github.com/NetSys/quilt/api"
)

type containerCommand struct {
	host string
}

func (cCmd *containerCommand) getFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("queryContainers", flag.ExitOnError)

	flags.StringVar(&cCmd.host, "H", api.DefaultSocket, "the host to connect to")

	return flags
}

func (cCmd *containerCommand) run() {
	client, err := api.NewClient(cCmd.host)
	if err != nil {
		fmt.Printf(err.Error())
	}

	allContainers, err := client.QueryContainers()
	if err != nil {
		fmt.Println(err.Error())
	}

	fmt.Printf("Containers: %v\n", allContainers)
}
