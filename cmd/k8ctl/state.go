package main

import (
	"flag"

	"k8/internal/client"
)

func runState(args []string) {
	fs := flag.NewFlagSet("state", flag.ExitOnError)
	server := serverFlag(fs)
	asJSON := fs.Bool("json", false, "output raw JSON")
	fs.Parse(args)

	state, err := client.New(*server).FetchState()
	if err != nil {
		exitErr(err)
	}

	if *asJSON {
		printJSON(state)
		return
	}
	printState(state)
}
