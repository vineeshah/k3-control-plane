package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func serverFlag(fs *flag.FlagSet) *string {
	return fs.String("server", "http://127.0.0.1:8080", "controller base URL")
}

func parseMap(input string) map[string]string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	values := make(map[string]string)
	for _, pair := range strings.Split(input, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		values[parts[0]] = parts[1]
	}
	return values
}

func parsePorts(input string) ([]int, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}
	var ports []int
	for _, raw := range strings.Split(input, ",") {
		port, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid port %q", raw)
		}
		ports = append(ports, port)
	}
	return ports, nil
}

func splitCommand(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	return strings.Fields(input)
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
