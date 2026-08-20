package main

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

type Alert struct {
	SourceName string
	Path       string
	Offset     int64
	ObservedAt time.Time
	Lines      []string
}

type sourceTailer struct {
	cfg           *Config
	source        SourceConfig
	store         *StateStore
	matcher       *Matcher
	alerts        chan<- Alert
	logger        *log.Logger
	location      *time.Location
	currentPath   string
	offset        int64
	partial       string
	partialOffset int64
	pendingAlert  *Alert
	pendingLeft   int
	openedOnce    bool
}

func TailSource(ctx context.Context, cfg *Config, source SourceConfig, store *StateStore, matcher *Matcher, alerts chan<- Alert, logger *log.Logger, once bool) {
	loc, err := source.Location()
	if err != nil {
		logger.Printf("[%s] load timezone failed: %v", source.Name, err)
		return
	}

	t := &sourceTailer{
		cfg:      cfg,
		source:   source,
		store:    store,
		matcher:  matcher,
		alerts:   alerts,
		logger:   logger,
		location: loc,
	}

	ticker := time.NewTicker(cfg.PollDuration())
	defer ticker.Stop()
	defer t.flushPending()

	for {
		t.poll(ctx)
		if once {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (t *sourceTailer) poll(ctx context.Context) {
	now := time.Now().In(t.location)
	path := t.source.CurrentPath(now)
	if path != t.currentPath {
		t.switchPath(path)
	}

	f, err := os.Open(t.currentPath)
	if err != nil {
		if !os.IsNotExist(err) {
			t.logger.Printf("[%s] open %s failed: %v", t.source.Name, t.currentPath, err)
		}
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.logger.Printf("[%s] stat %s failed: %v", t.source.Name, t.currentPath, err)
		return
	}
	size := info.Size()
	if size < t.offset {
		t.offset = 0
		t.partial = ""
		t.partialOffset = 0
		t.flushPending()
	}
	if size == t.offset {
		return
	}

	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		t.logger.Printf("[%s] seek %s failed: %v", t.source.Name, t.currentPath, err)
		return
	}

	buf := make([]byte, 64*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			t.processBytes(buf[:n], t.offset)
			t.offset += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			t.logger.Printf("[%s] read %s failed: %v", t.source.Name, t.currentPath, readErr)
			break
		}
	}

	t.store.Set(t.source.Name, t.currentPath, t.offset, size)
	if err := t.store.Save(); err != nil {
		t.logger.Printf("[%s] save state failed: %v", t.source.Name, err)
	}
}

func (t *sourceTailer) switchPath(path string) {
	t.flushPending()
	t.partial = ""
	t.partialOffset = 0

	var offset int64
	if state, ok := t.store.Get(t.source.Name, path); ok {
		offset = state.Offset
	} else if t.openedOnce && t.currentPath != "" && t.source.DirectFile == "" {
		offset = 0
	} else if t.cfg.ShouldStartAtEnd() {
		if info, err := os.Stat(path); err == nil {
			offset = info.Size()
			t.store.Set(t.source.Name, path, offset, info.Size())
			if err := t.store.Save(); err != nil {
				t.logger.Printf("[%s] save initial state failed: %v", t.source.Name, err)
			}
		}
	}

	t.currentPath = path
	t.offset = offset
	t.openedOnce = true
	t.logger.Printf("[%s] watching %s from offset %d", t.source.Name, path, offset)
}

func (t *sourceTailer) processBytes(data []byte, chunkOffset int64) {
	text := t.partial + string(data)
	parts := strings.SplitAfter(text, "\n")
	if len(parts) == 0 {
		return
	}

	textOffset := chunkOffset
	if t.partial != "" {
		textOffset = t.partialOffset
	}

	if strings.HasSuffix(text, "\n") {
		t.partial = ""
		t.partialOffset = 0
	} else {
		t.partial = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}

	lineOffset := textOffset
	for _, part := range parts {
		line := strings.TrimRight(part, "\r\n")
		t.processLine(line, lineOffset)
		lineOffset += int64(len(part))
	}
	if t.partial != "" {
		t.partialOffset = lineOffset
	}
}

func (t *sourceTailer) processLine(line string, lineOffset int64) {
	if t.pendingAlert != nil {
		t.pendingAlert.Lines = append(t.pendingAlert.Lines, line)
		t.pendingLeft--
		if t.pendingLeft <= 0 {
			t.emitPending()
		}
		return
	}

	if !t.matcher.Match(line) {
		return
	}

	alert := &Alert{
		SourceName: t.source.Name,
		Path:       t.currentPath,
		Offset:     lineOffset,
		ObservedAt: time.Now(),
		Lines:      []string{line},
	}
	if t.matcher.ContextLinesAfter() == 0 {
		t.alerts <- *alert
		return
	}
	t.pendingAlert = alert
	t.pendingLeft = t.matcher.ContextLinesAfter()
}

func (t *sourceTailer) flushPending() {
	if t.pendingAlert != nil {
		t.emitPending()
	}
}

func (t *sourceTailer) emitPending() {
	if t.pendingAlert == nil {
		return
	}
	t.alerts <- *t.pendingAlert
	t.pendingAlert = nil
	t.pendingLeft = 0
}
