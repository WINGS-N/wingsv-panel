package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"v.wingsnet.org/internal/config"
	"v.wingsnet.org/internal/httpapi"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("v.wingsnet.org listening on %s", cfg.ListenAddr)
	if err := httpapi.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
