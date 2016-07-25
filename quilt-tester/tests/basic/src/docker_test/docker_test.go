package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func main() {
	output, err := exec.Command("docker", "ps", "-a").Output()
	if err != nil {
		fmt.Println(output)
		panic(err)
	}

	outputStr := string(output)
	fmt.Println(outputStr)
	if strings.Contains(outputStr, "Exited") {
		fmt.Println("FAILED, containers were exited")
	} else {
		fmt.Println("PASSED")
	}
}
