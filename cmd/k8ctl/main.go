package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "service":
		runService(os.Args[2:])
	case "job":
		runJob(os.Args[2:])
	case "get":
		runGet(os.Args[2:])
	case "state":
		runState(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("usage:")
	fmt.Println("  k8ctl service create -name web -image nginx -replicas 2 -cpu 100 -memory 256")
	fmt.Println("  k8ctl service delete web")
	fmt.Println("  k8ctl job create -name backup -image alpine -command \"backup now\" -retries 1")
	fmt.Println("  k8ctl job delete backup")
	fmt.Println("  k8ctl get [--json] nodes|services|jobs|assignments")
	fmt.Println("  k8ctl state [--json]")
}
