package main

import (
	"context"
	"errors"
	"io"
	"log"
	"sync/atomic"
	"testing"
	"time"
)

type failingSender struct {
	calls int32
}

func (s *failingSender) Send(ctx context.Context, text string) error {
	atomic.AddInt32(&s.calls, 1)
	return errors.New("telegram unavailable")
}

func (s *failingSender) MaxChars() int {
	return 3500
}

func TestRunBatcherOpensCircuitAndStopsRetrying(t *testing.T) {
	alerts := make(chan Alert, 3)
	alerts <- Alert{SourceName: "server-1", Lines: []string{`{"level":"ERROR","msg":"one"}`}}
	alerts <- Alert{SourceName: "server-1", Lines: []string{`{"level":"ERROR","msg":"two"}`}}
	alerts <- Alert{SourceName: "server-1", Lines: []string{`{"level":"ERROR","msg":"three"}`}}
	close(alerts)

	sender := &failingSender{}
	done := make(chan struct{})
	go func() {
		RunBatcher(context.Background(), alerts, sender, 10*time.Millisecond, time.Millisecond, 1, log.New(io.Discard, "", 0))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunBatcher did not finish")
	}

	if got := atomic.LoadInt32(&sender.calls); got != 2 {
		t.Fatalf("expected 2 send attempts before circuit opened, got %d", got)
	}
}
