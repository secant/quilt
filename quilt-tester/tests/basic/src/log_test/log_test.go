package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func main() {
	output, err := exec.Command("docker", "logs", "minion").CombinedOutput()
	if err != nil {
		fmt.Println(output)
		panic(err)
	}

	outputStr := string(output)
	fmt.Println(outputStr)
	if strings.Contains(outputStr, "ERROR") || strings.Contains(outputStr, "WARN") {
		fmt.Println("FAILED")
	} else {
		fmt.Println("PASSED")
	}

}
