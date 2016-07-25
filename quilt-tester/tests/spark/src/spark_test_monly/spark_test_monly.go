package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// Grab the container with master
	output, err := exec.Command("swarm", "ps", "-a").Output()
	if err != nil {
		panic(err)
	}

	containerStr := string(output)
	fmt.Println("Output of swarm ps -a:")
	fmt.Println(containerStr)

	containers := strings.Split(containerStr, "\n")

	var master string
	for _, line := range containers {
		if strings.Contains(line, "run master") {
			master = strings.Fields(line)[0]
		}
	}

	if master == "" {
		fmt.Println("FAILED, no spark master node was found.")
		os.Exit(1)
	}

	output, err = exec.Command("swarm", "logs", master).CombinedOutput()
	masterLog := string(output)

	if err != nil {
		panic(err)
	}

	fmt.Println("Output of swarm logs <master node>:")
	fmt.Println(masterLog)

	if !strings.Contains(masterLog, "Pi is roughly") {
		fmt.Println("FAILED, sparkPI did not execute correctly.")
	} else {
		fmt.Println("PASSED")
	}
}
