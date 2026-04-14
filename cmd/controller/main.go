package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"k8/internal/controller"
	"k8/internal/scheduler"
	"k8/internal/store"
)

func main() {
	listenAddr := flag.String("listen", ":8080", "controller listen address")
	reconcileInterval := flag.Duration("reconcile-interval", 2*time.Second, "how often to reconcile desired state")
	nodeTimeout := flag.Duration("node-timeout", 10*time.Second, "when a node is considered offline")
	flag.Parse()

	state := store.NewMemoryStore()
	sched := scheduler.New(*nodeTimeout)
	ctrl := controller.New(state, sched, *nodeTimeout)
	server := controller.NewHTTPServer(ctrl)

	go func() {
		ticker := time.NewTicker(*reconcileInterval)
		defer ticker.Stop()

		ctrl.Reconcile(time.Now().UTC())
		for tick := range ticker.C {
			ctrl.Reconcile(tick.UTC())
		}
	}()

	log.Printf("controller listening on %s", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
