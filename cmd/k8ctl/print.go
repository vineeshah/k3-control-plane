package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"k8/internal/api"
)

func printJSON(v any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		exitErr(err)
	}
}

func printState(state api.StateSnapshot) {
	printNodes(state.Nodes)
	printServices(state.Services)
	printJobs(state.Jobs)
	printAssignments(state.Assignments)
}

func printNodes(nodes []api.Node) {
	if len(nodes) == 0 {
		return
	}
	fmt.Println("NODES")
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tLABELS\tCPU\tMEMORY\tHEARTBEAT")
	for _, node := range nodes {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n",
			node.ID, formatMap(node.Labels), node.Capacity.CPU, node.Capacity.Memory, age(node.LastHeartbeat))
	}
	w.Flush()
	fmt.Println()
}

func printServices(services []api.Service) {
	if len(services) == 0 {
		return
	}
	fmt.Println("SERVICES")
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tIMAGE\tREPLICAS\tRUNNING\tPENDING\tASSIGNMENTS")
	for _, service := range services {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%s\n",
			service.Name, service.Image, service.Status.DesiredReplicas,
			service.Status.RunningReplicas, service.Status.PendingReplicas,
			strings.Join(service.Status.AssignmentIDs, ","))
	}
	w.Flush()
	fmt.Println()
}

func printJobs(jobs []api.Job) {
	if len(jobs) == 0 {
		return
	}
	fmt.Println("JOBS")
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tIMAGE\tATTEMPTS\tSTATUS")
	for _, job := range jobs {
		status := "running"
		if job.Status.Succeeded {
			status = "succeeded"
		} else if job.Status.FailedAttempts > job.Retries {
			status = "failed"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", job.Name, job.Image, job.Status.FailedAttempts, status)
	}
	w.Flush()
	fmt.Println()
}

func printAssignments(assignments []api.Assignment) {
	if len(assignments) == 0 {
		return
	}
	fmt.Println("ASSIGNMENTS")
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tOWNER\tNODE\tPHASE\tIMAGE\tCPU\tMEMORY")
	for _, assignment := range assignments {
		fmt.Fprintf(w, "%s\t%s/%s\t%s\t%s\t%s\t%d\t%d\n",
			assignment.ID, assignment.OwnerKind, assignment.OwnerName, assignment.NodeID,
			assignment.Phase, assignment.Image, assignment.Resources.CPU, assignment.Resources.Memory)
	}
	w.Flush()
	fmt.Println()
}

func formatMap(values map[string]string) string {
	if len(values) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+values[key])
	}
	return strings.Join(pairs, ",")
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
