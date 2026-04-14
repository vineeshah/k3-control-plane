package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"k8/internal/api"
	"k8/internal/client"
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
	case "state":
		runState(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func runService(args []string) {
	if len(args) == 0 || args[0] != "create" {
		usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("service create", flag.ExitOnError)
	server := fs.String("server", "http://127.0.0.1:8080", "controller base URL")
	name := fs.String("name", "", "service name")
	image := fs.String("image", "", "container image")
	command := fs.String("command", "", "space-delimited command")
	replicas := fs.Int("replicas", 1, "desired replica count")
	cpu := fs.Int("cpu", 100, "cpu reservation")
	memory := fs.Int("memory", 128, "memory reservation in MB")
	env := fs.String("env", "", "comma-separated KEY=value entries")
	labels := fs.String("labels", "", "comma-separated requiredLabel=value placement constraints")
	fs.Parse(args[1:])

	service := api.Service{
		Name:     *name,
		Image:    *image,
		Command:  splitCommand(*command),
		Env:      parseMap(*env),
		Replicas: *replicas,
		Resources: api.ResourceRequirements{
			CPU:    *cpu,
			Memory: *memory,
		},
		Placement: api.Placement{
			RequiredLabels: parseMap(*labels),
		},
	}

	if err := client.New(*server).ApplyService(service); err != nil {
		exitErr(err)
	}
	fmt.Printf("service %q applied\n", service.Name)
}

func runJob(args []string) {
	if len(args) == 0 || args[0] != "create" {
		usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("job create", flag.ExitOnError)
	server := fs.String("server", "http://127.0.0.1:8080", "controller base URL")
	name := fs.String("name", "", "job name")
	image := fs.String("image", "", "container image")
	command := fs.String("command", "", "space-delimited command")
	retries := fs.Int("retries", 0, "retry count after the first failed attempt")
	cpu := fs.Int("cpu", 100, "cpu reservation")
	memory := fs.Int("memory", 128, "memory reservation in MB")
	env := fs.String("env", "", "comma-separated KEY=value entries")
	labels := fs.String("labels", "", "comma-separated requiredLabel=value placement constraints")
	fs.Parse(args[1:])

	job := api.Job{
		Name:    *name,
		Image:   *image,
		Command: splitCommand(*command),
		Env:     parseMap(*env),
		Retries: *retries,
		Resources: api.ResourceRequirements{
			CPU:    *cpu,
			Memory: *memory,
		},
		Placement: api.Placement{
			RequiredLabels: parseMap(*labels),
		},
	}

	if err := client.New(*server).ApplyJob(job); err != nil {
		exitErr(err)
	}
	fmt.Printf("job %q applied\n", job.Name)
}

func runState(args []string) {
	fs := flag.NewFlagSet("state", flag.ExitOnError)
	server := fs.String("server", "http://127.0.0.1:8080", "controller base URL")
	fs.Parse(args)

	state, err := client.New(*server).FetchState()
	if err != nil {
		exitErr(err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		exitErr(err)
	}
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

func usage() {
	fmt.Println("usage:")
	fmt.Println("  k8ctl service create -name web -image nginx -replicas 2")
	fmt.Println("  k8ctl job create -name backup -image alpine -command \"backup now\" -retries 1")
	fmt.Println("  k8ctl state")
}
