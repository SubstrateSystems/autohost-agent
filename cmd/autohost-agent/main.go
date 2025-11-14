package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"autohost-agent/internal/agent"
	"autohost-agent/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: autohost-agent <config-path>")
	}
	cfgPath := os.Args[1]

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	a := agent.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := a.Run(ctx); err != nil {
		log.Fatalf("agent stopped: %v", err)
	}
}
