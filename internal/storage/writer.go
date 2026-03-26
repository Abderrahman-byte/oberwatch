package storage

import (
	"context"
	"log/slog"
	"sync"
)

// BufferedCostWriter asynchronously persists cost records through a buffered channel.
type BufferedCostWriter struct {
	store  Store
	logger *slog.Logger

	ch chan CostRecord
	wg sync.WaitGroup
}

// NewBufferedCostWriter creates and starts a buffered cost writer worker.
func NewBufferedCostWriter(store Store, bufferSize int, logger *slog.Logger) *BufferedCostWriter {
	if bufferSize < 1 {
		bufferSize = 1
	}
	writer := &BufferedCostWriter{
		store:  store,
		logger: logger,
		ch:     make(chan CostRecord, bufferSize),
	}
	writer.wg.Add(1)
	go func() {
		defer writer.wg.Done()
		for record := range writer.ch {
			if err := writer.store.SaveCostRecord(context.Background(), record); err != nil && writer.logger != nil {
				writer.logger.Warn("async save cost record failed", "error", err)
			}
		}
	}()
	return writer
}

// Enqueue queues a cost record without blocking request paths.
func (w *BufferedCostWriter) Enqueue(record CostRecord) {
	if w == nil {
		return
	}
	select {
	case w.ch <- record:
	default:
		if w.logger != nil {
			w.logger.Warn("dropping cost record because writer buffer is full", "agent", record.Agent, "model", record.Model)
		}
	}
}

// Close stops the worker after draining queued records.
func (w *BufferedCostWriter) Close() {
	if w == nil {
		return
	}
	close(w.ch)
	w.wg.Wait()
}
