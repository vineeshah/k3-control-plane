package main

import "flag"

func runState(args []string) {
	fs := flag.NewFlagSet("state", flag.ExitOnError)
	config := configFlag(fs)
	asJSON := fs.Bool("json", false, "output raw JSON")
	fs.Parse(args)

	state, err := connect(*config).FetchState()
	if err != nil {
		exitErr(err)
	}

	if *asJSON {
		printJSON(state)
		return
	}
	printState(state)
}
