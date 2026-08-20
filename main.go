package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config json")
	once := flag.Bool("once", false, "poll once and exit")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	matcher, err := NewMatcher(cfg.Match)
	if err != nil {
		fmt.Fprintf(os.Stderr, "matcher error: %v\n", err)
		os.Exit(1)
	}

	store, err := LoadState(cfg.StateFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "state error: %v\n", err)
		os.Exit(1)
	}

	sender := NewTelegramClient(cfg.Telegram, cfg.HTTPTimeoutDuration(), cfg.MessageMaxChars, cfg.DryRun)
	alerts := make(chan Alert, 1024)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var tailWG sync.WaitGroup
	for _, source := range cfg.Sources {
		source := source
		tailWG.Add(1)
		go func() {
			defer tailWG.Done()
			TailSource(ctx, cfg, source, store, matcher, alerts, logger, *once)
		}()
	}

	var sendWG sync.WaitGroup
	sendWG.Add(1)
	go func() {
		defer sendWG.Done()
		RunBatcher(ctx, alerts, sender, cfg.SendDuration(), cfg.MaxBatchLines, logger)
	}()

	tailWG.Wait()
	close(alerts)
	sendWG.Wait()
	if err := store.Save(); err != nil {
		logger.Printf("final state save failed: %v", err)
	}
}
