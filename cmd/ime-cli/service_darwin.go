//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/zai/goime/internal/engine"
	"github.com/zai/goime/internal/macime"
)

func runServicePlatform(eng *engine.Engine) {
	server, err := macime.New(eng, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[service] create: %v\n", err)
		os.Exit(1)
	}
	server.OnRequest = func(method string) {
		fmt.Fprintf(os.Stderr, "[service] -> %s\n", method)
	}
	server.OnResponse = func(method string, success bool) {
		fmt.Fprintf(os.Stderr, "[service] <- %s (%v)\n", method, success)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "[service] shutting down...")
		server.Close()
		os.Exit(0)
	}()

	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "[service] serve: %v\n", err)
		os.Exit(1)
	}
}
