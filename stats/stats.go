package stats

import (
	"context"
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

func (s *Stats) RecordConnection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeConns++
	s.totalConns++
}

func (s *Stats) RecordDisconnection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeConns--
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

func (s *Stats) GetStorageStats(ctx context.Context, allowedKinds []int) map[int]int64 {
	result := make(map[int]int64)

	for _, kind := range allowedKinds {
		count, err := s.storage.CountEvents(ctx, kind)
		if err == nil {
			result[kind] = count
		}
	}

	return result
}
