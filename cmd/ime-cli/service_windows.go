//go:build windows

// Windows 平台：启动命名管道服务
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/zai/goime/internal/engine"
	"github.com/zai/goime/internal/winime"
)

func runServicePlatform(eng *engine.Engine) {
	server, err := winime.NewPipeServer(eng, `\\.\pipe\goime`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[service] create pipe server: %v\n", err)
		os.Exit(1)
	}

	server.OnRequest = func(method string) {
		fmt.Fprintf(os.Stderr, "[service] -> %s\n", method)
	}
	server.OnResponse = func(method string, success bool) {
		fmt.Fprintf(os.Stderr, "[service] <- %s (%v)\n", method, success)
	}

	// 信号处理
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
