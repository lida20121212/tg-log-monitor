package monitor

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func Run(args []string) error {
	fs := flag.NewFlagSet("tg-log-monitor", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "path to config json")
	once := fs.Bool("once", false, "poll once and exit")
	testTelegram := fs.Bool("test-telegram", false, "send one Telegram test message and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	sender := NewTelegramClient(cfg.Telegram, cfg.HTTPTimeoutDuration(), cfg.MessageMaxChars, cfg.DryRun)
	if *testTelegram {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTPTimeoutDuration())
		defer cancel()
		text := fmt.Sprintf("[TG LOG MONITOR TEST]\ntime: %s\nconfig: %s\nsources: %d",
			time.Now().Format("2006-01-02 15:04:05"), *configPath, len(cfg.Sources))
		if err := sender.Send(ctx, text); err != nil {
			return fmt.Errorf("telegram test failed: %w", err)
		}
		logger.Printf("telegram test message sent")
		return nil
	}

	matcher, err := NewMatcher(cfg.Match)
	if err != nil {
		return fmt.Errorf("matcher error: %w", err)
	}

	store, err := LoadState(cfg.StateFile)
	if err != nil {
		return fmt.Errorf("state error: %w", err)
	}

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
		RunBatcher(ctx, alerts, sender, cfg.SendDuration(), cfg.HTTPTimeoutDuration(), cfg.MaxBatchLines, logger)
	}()

	tailWG.Wait()
	close(alerts)
	sendWG.Wait()
	if err := store.Save(); err != nil {
		logger.Printf("final state save failed: %v", err)
	}
	return nil
}
