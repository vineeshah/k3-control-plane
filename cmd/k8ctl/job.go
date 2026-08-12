package main

import (
	"flag"
	"fmt"
	"os"

	"k8/internal/api"
	"k8/internal/client"
)

func runJob(args []string) {
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	switch args[0] {
	case "create":
		runJobCreate(args[1:])
	case "delete":
		runJobDelete(args[1:])
	default:
		usage()
		os.Exit(1)
	}
}

func runJobCreate(args []string) {
	fs := flag.NewFlagSet("job create", flag.ExitOnError)
	server := serverFlag(fs)
	name := fs.String("name", "", "job name")
	image := fs.String("image", "", "container image")
	command := fs.String("command", "", "space-delimited command")
	retries := fs.Int("retries", 0, "retry count after the first failed attempt")
	cpu := fs.Int("cpu", 100, "cpu reservation")
	memory := fs.Int("memory", 128, "memory reservation in MB")
	env := fs.String("env", "", "comma-separated KEY=value entries")
	labels := fs.String("labels", "", "comma-separated requiredLabel=value placement constraints")
	fs.Parse(args)

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

func runJobDelete(args []string) {
	fs := flag.NewFlagSet("job delete", flag.ExitOnError)
	server := serverFlag(fs)
	fs.Parse(args)

	if fs.NArg() != 1 {
		usage()
		os.Exit(1)
	}
	if err := client.New(*server).DeleteJob(fs.Arg(0)); err != nil {
		exitErr(err)
	}
	fmt.Printf("job %q deleted\n", fs.Arg(0))
}
