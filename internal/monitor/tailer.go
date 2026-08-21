package monitor

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

type Alert struct {
	Kind       AlertKind
	SourceName string
	Path       string
	Offset     int64
	ObservedAt time.Time
	Lines      []string
	Resource   *ResourceAlert
}

type sourceWatcher struct {
	cfg             *Config
	source          SourceConfig
	store           *StateStore
	matcher         *Matcher
	alerts          chan<- Alert
	logger          *log.Logger
	location        *time.Location
	tailers         map[string]*sourceTailer
	initialScanDone bool
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
	droppedAlerts int
}

func TailSource(ctx context.Context, cfg *Config, source SourceConfig, store *StateStore, matcher *Matcher, alerts chan<- Alert, logger *log.Logger, once bool) {
	loc, err := source.Location()
	if err != nil {
		logger.Printf("[%s] load timezone failed: %v", source.Name, err)
		return
	}

	w := &sourceWatcher{
		cfg:      cfg,
		source:   source,
		store:    store,
		matcher:  matcher,
		alerts:   alerts,
		logger:   logger,
		location: loc,
		tailers:  map[string]*sourceTailer{},
	}

	ticker := time.NewTicker(cfg.PollDuration())
	defer ticker.Stop()
	defer w.flushPending()

	for {
		w.poll(ctx)
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

func (w *sourceWatcher) poll(ctx context.Context) {
	now := time.Now().In(w.location)
	paths, err := w.source.CurrentPaths(now)
	if err != nil {
		w.logger.Printf("[%s] resolve log files failed: %v", w.source.Name, err)
		return
	}

	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		seen[path] = true
		tailer := w.tailers[path]
		if tailer == nil {
			tailer = w.newTailer(path)
			w.tailers[path] = tailer
		}
		tailer.poll(ctx)
	}

	for path, tailer := range w.tailers {
		if !seen[path] {
			tailer.flushPending()
			delete(w.tailers, path)
		}
	}
	w.initialScanDone = true
}

func (w *sourceWatcher) newTailer(path string) *sourceTailer {
	var offset int64
	if state, ok := w.store.Get(w.source.Name, path); ok {
		offset = state.Offset
	} else if w.cfg.ShouldStartAtEnd() && !w.initialScanDone {
		if info, err := os.Stat(path); err == nil {
			offset = info.Size()
			w.store.Set(w.source.Name, path, offset, info.Size())
			if err := w.store.Save(); err != nil {
				w.logger.Printf("[%s] save initial state failed: %v", w.source.Name, err)
			}
		}
	}

	w.logger.Printf("[%s] watching %s from offset %d", w.source.Name, path, offset)
	return &sourceTailer{
		cfg:         w.cfg,
		source:      w.source,
		store:       w.store,
		matcher:     w.matcher,
		alerts:      w.alerts,
		logger:      w.logger,
		location:    w.location,
		currentPath: path,
		offset:      offset,
	}
}

func (w *sourceWatcher) flushPending() {
	for _, tailer := range w.tailers {
		tailer.flushPending()
	}
}

func (t *sourceTailer) poll(ctx context.Context) {
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
		t.enqueueAlert(*alert)
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
	t.enqueueAlert(*t.pendingAlert)
	t.pendingAlert = nil
	t.pendingLeft = 0
}

func (t *sourceTailer) enqueueAlert(alert Alert) bool {
	select {
	case t.alerts <- alert:
		return true
	default:
		t.droppedAlerts++
		if t.droppedAlerts == 1 || t.droppedAlerts%100 == 0 {
			t.logger.Printf("[%s] alert queue full, dropped %d alert(s)", t.source.Name, t.droppedAlerts)
		}
		return false
	}
}
