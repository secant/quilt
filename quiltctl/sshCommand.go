package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/NetSys/quilt/api"
)

type sshCommand struct {
	host string

	flags *flag.FlagSet
}

func (sCmd *sshCommand) getFlagSet() *flag.FlagSet {
	sCmd.flags = flag.NewFlagSet("ssh", flag.ExitOnError)

	sCmd.flags.StringVar(&sCmd.host, "host", api.DefaultSocket,
		"the host to query for machine information")

	return sCmd.flags
}

func (sCmd *sshCommand) run() {
	args := sCmd.flags.Args()
	tgtMachine, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Printf("bad machine: %s", err.Error())
		return
	}
	sshArgs := args[1:]

	c, err := api.NewClient(sCmd.host)
	if err != nil {
		fmt.Println(err.Error())
	}

	machines, err := c.QueryMachines()
	if err != nil {
		fmt.Println(err)
	}

	var host string
	for _, m := range machines {
		if m.ID == tgtMachine {
			host = m.PublicIP
			break
		}
	}

	if host == "" {
		fmt.Println("Could not find machine.")
		// XXX: exit
	}

	baseArgs := []string{"-o", "UserKnownHostsFile=/dev/null", "-o",
		"StrictHostKeyChecking=no", fmt.Sprintf("quilt@%s", host)}

	cmd := exec.Command("ssh", append(baseArgs, sshArgs...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err = cmd.Run(); err != nil {
		fmt.Println(err.Error())
	}
}
