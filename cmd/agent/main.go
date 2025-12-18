package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"autohost-agent/internal/agent"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) < 2 {
		log.Fatal("usage: autohost-agent <config-path>")
	}
	cfgPath := os.Args[1]

	log.Printf("Loading configuration from: %s", cfgPath)
	cfg, err := agent.Load(cfgPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	a := agent.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("Starting Autohost Agent...")
	if err := a.Run(ctx); err != nil {
		log.Fatalf("agent stopped: %v", err)
	}
	log.Println("Agent stopped gracefully")
}
