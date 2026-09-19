package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"k8/internal/client"
)

func configFlag(fs *flag.FlagSet) *string {
	return fs.String("config", defaultConfigPath(), "admin config written by the controller (also $K8_CONFIG)")
}

func defaultConfigPath() string {
	if path := os.Getenv("K8_CONFIG"); path != "" {
		return path
	}
	return "/var/lib/k8/admin.conf"
}

func connect(configPath string) *client.Client {
	c, err := client.LoadAdminConfig(configPath)
	if err != nil {
		exitErr(err)
	}
	return c
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
