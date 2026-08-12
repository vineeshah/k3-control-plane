package main

import (
	"flag"
	"fmt"
	"os"

	"k8/internal/api"
	"k8/internal/client"
)

func runService(args []string) {
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	switch args[0] {
	case "create":
		runServiceCreate(args[1:])
	case "delete":
		runServiceDelete(args[1:])
	default:
		usage()
		os.Exit(1)
	}
}

func runServiceCreate(args []string) {
	fs := flag.NewFlagSet("service create", flag.ExitOnError)
	server := serverFlag(fs)
	name := fs.String("name", "", "service name")
	image := fs.String("image", "", "container image")
	command := fs.String("command", "", "space-delimited command")
	replicas := fs.Int("replicas", 1, "desired replica count")
	cpu := fs.Int("cpu", 100, "cpu reservation")
	memory := fs.Int("memory", 128, "memory reservation in MB")
	env := fs.String("env", "", "comma-separated KEY=value entries")
	labels := fs.String("labels", "", "comma-separated requiredLabel=value placement constraints")
	fs.Parse(args)

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

func runServiceDelete(args []string) {
	fs := flag.NewFlagSet("service delete", flag.ExitOnError)
	server := serverFlag(fs)
	fs.Parse(args)

	if fs.NArg() != 1 {
		usage()
		os.Exit(1)
	}
	if err := client.New(*server).DeleteService(fs.Arg(0)); err != nil {
		exitErr(err)
	}
	fmt.Printf("service %q deleted\n", fs.Arg(0))
}
