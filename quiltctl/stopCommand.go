package main

import (
	"flag"
	"fmt"

	"github.com/NetSys/quilt/api"
)

type stopCommand struct {
	host      string
	namespace string
}

func (sCmd *stopCommand) getFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("stop", flag.ExitOnError)

	flags.StringVar(&sCmd.host, "host", api.DefaultSocket, "the host to connect to")
	flags.StringVar(&sCmd.namespace, "namespace", "", "the namespace to stop")

	return flags
}

func (sCmd *stopCommand) run() {
	// XXX: Get the stitch from rCmd.flags if rCmd.host is empty.
	specStr := "(define AdminACL (list))"
	if sCmd.namespace != "" {
		specStr += fmt.Sprintf(` (define Namespace "%s")`, sCmd.namespace)
	}

	client, err := api.NewClient(sCmd.host)
	if err != nil {
		fmt.Printf(err.Error())
	}

	err = client.RunStitch(specStr)
	if err != nil {
		fmt.Println(err.Error())
	}
}
