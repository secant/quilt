package util

import (
	"fmt"
	"strings"
)

// ParseListenAddress validates and parses a socket address.
func ParseListenAddress(lAddr string) (string, string, error) {
	addrParts := strings.Split(lAddr, "://")
	if len(addrParts) != 2 || (addrParts[0] != "unix" && addrParts[0] != "tcp") {
		return "", "", fmt.Errorf("malformed listen address: %s", lAddr)
	}
	return addrParts[0], addrParts[1], nil
}
