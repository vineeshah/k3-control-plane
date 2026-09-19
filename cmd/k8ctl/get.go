package main

import (
	"os"
	"strings"

	"k8/internal/api"
)

func runGet(args []string) {
	config := defaultConfigPath()
	asJSON := false
	resource := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-config" || arg == "--config":
			if i+1 < len(args) {
				i++
				config = args[i]
			}
		case strings.HasPrefix(arg, "-config=") || strings.HasPrefix(arg, "--config="):
			config = strings.SplitN(arg, "=", 2)[1]
		case arg == "-json" || arg == "--json":
			asJSON = true
		default:
			if resource == "" {
				resource = arg
			}
		}
	}

	if resource == "" {
		usage()
		os.Exit(1)
	}

	state, err := connect(config).FetchState()
	if err != nil {
		exitErr(err)
	}

	switch resource {
	case "nodes":
		printOrJSON(asJSON, state.Nodes)
	case "services":
		printOrJSON(asJSON, state.Services)
	case "jobs":
		printOrJSON(asJSON, state.Jobs)
	case "assignments":
		printOrJSON(asJSON, state.Assignments)
	default:
		usage()
		os.Exit(1)
	}
}

func printOrJSON(asJSON bool, v any) {
	if asJSON {
		printJSON(v)
		return
	}
	switch typed := v.(type) {
	case []api.Node:
		printNodes(typed)
	case []api.Service:
		printServices(typed)
	case []api.Job:
		printJobs(typed)
	case []api.Assignment:
		printAssignments(typed)
	}
}
