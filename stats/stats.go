package stats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pablof7z/purplepag.es/storage"
)

type Stats struct {
	mu             sync.RWMutex
	startTime      time.Time
	totalEvents    int64
	eventsByKind   map[int]int64
	acceptedEvents int64
	rejectedEvents int64
	activeConns    int64
	totalConns     int64
	storage        *storage.Storage
}

func New(storage *storage.Storage) *Stats {
	return &Stats{
		startTime:    time.Now(),
		eventsByKind: make(map[int]int64),
		storage:      storage,
	}
}

func (s *Stats) RecordEventAccepted(kind int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acceptedEvents++
	s.totalEvents++
	s.eventsByKind[kind]++
}

func (s *Stats) RecordEventRejected() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectedEvents++
}

func (s *Stats) RecordEventRejectedForKind(ctx context.Context, kind int, pubkey string) {
	s.mu.Lock()
	s.rejectedEvents++
	s.mu.Unlock()

	s.storage.RecordRejectedEvent(ctx, kind, pubkey)
}

func (s *Stats) RecordRejectedREQ(ctx context.Context, kind int) {
	s.storage.RecordRejectedREQ(ctx, kind)
}

func (s *Stats) RecordREQKind(ctx context.Context, kind int) {
	s.storage.RecordREQKind(ctx, kind)
}

func (s *Stats) RecordConnection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeConns++
	s.totalConns++
}

func (s *Stats) RecordDisconnection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConns > 0 {
		s.activeConns--
	}
}

func (s *Stats) GetUptime() time.Duration {
	return time.Since(s.startTime)
}

func (s *Stats) GetEventsByKind() map[int]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[int]int64)
	for k, v := range s.eventsByKind {
		result[k] = v
	}
	return result
}

func (s *Stats) GetTotalEvents() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalEvents
}

func (s *Stats) GetAcceptedEvents() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.acceptedEvents
}

func (s *Stats) GetRejectedEvents() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rejectedEvents
}

func (s *Stats) GetActiveConnections() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeConns
}

func (s *Stats) GetTotalConnections() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalConns
}

func (s *Stats) GetDiscoveredRelayCount(ctx context.Context) int64 {
	count, err := s.storage.GetDiscoveredRelayCount(ctx)
	if err != nil {
		return 0
	}
	return count
}

func (s *Stats) GetStorageStats(ctx context.Context) map[int]int64 {
	result, err := s.storage.GetEventCountsByKind(ctx)
	if err != nil || result == nil {
		return make(map[int]int64)
	}
	return result
}

func (s *Stats) RecordEventsServed(ctx context.Context, ip string, eventsCount int64) {
	if err := s.storage.RecordDailyStats(ctx, ip, eventsCount); err != nil {
		// Silently ignore errors for now
	}
}

// FormatBytes converts a byte count to a human-readable string.
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// FormatNumber converts a large number to a human-readable string with K/M suffixes.
func FormatNumber(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.2fM", float64(n)/1000000)
	} else if n >= 1000 {
		return fmt.Sprintf("%.2fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
