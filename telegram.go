package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Sender interface {
	Send(ctx context.Context, text string) error
	MaxChars() int
}

type TelegramClient struct {
	token     string
	chatID    int64
	parseMode string
	client    *http.Client
	maxChars  int
	dryRun    bool
}

func NewTelegramClient(cfg TelegramConfig, timeout time.Duration, maxChars int, dryRun bool) *TelegramClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if maxChars <= 0 || maxChars > 4000 {
		maxChars = 3500
	}
	return &TelegramClient{
		token:     strings.TrimSpace(cfg.BotToken),
		chatID:    cfg.ChatID.Int64(),
		parseMode: strings.TrimSpace(cfg.ParseMode),
		client:    &http.Client{Timeout: timeout},
		maxChars:  maxChars,
		dryRun:    dryRun,
	}
}

func (c *TelegramClient) MaxChars() int {
	return c.maxChars
}

func (c *TelegramClient) Send(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if c.dryRun {
		fmt.Printf("\n--- DRY RUN TELEGRAM MESSAGE ---\n%s\n--- END MESSAGE ---\n", text)
		return nil
	}
	if c.token == "" {
		return fmt.Errorf("telegram bot token is empty")
	}
	if c.chatID == 0 {
		return fmt.Errorf("telegram chat id is empty")
	}

	form := url.Values{}
	form.Set("chat_id", fmt.Sprintf("%d", c.chatID))
	form.Set("text", text)
	if c.parseMode != "" {
		form.Set("parse_mode", c.parseMode)
	}

	apiURL := "https://api.telegram.org/bot" + c.token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram api status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func RunBatcher(ctx context.Context, alerts <-chan Alert, sender Sender, interval time.Duration, maxBatch int, logger *log.Logger) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if maxBatch <= 0 {
		maxBatch = 20
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()

	var batch []Alert
	flush := func() {
		if len(batch) == 0 {
			return
		}
		messages := BuildMessages(batch, sender.MaxChars())
		for _, msg := range messages {
			sendCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := sender.Send(sendCtx, msg); err != nil {
				logger.Printf("telegram send failed: %v", err)
			}
			cancel()
		}
		batch = batch[:0]
	}

	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(interval)
	}

	for {
		select {
		case alert, ok := <-alerts:
			if !ok {
				flush()
				return
			}
			batch = append(batch, alert)
			if len(batch) >= maxBatch {
				flush()
				resetTimer()
			}
		case <-timer.C:
			flush()
			timer.Reset(interval)
		case <-ctx.Done():
			flush()
			return
		}
	}
}
