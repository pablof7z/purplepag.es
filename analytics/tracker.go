package analytics

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/purplepages/relay/storage"
)

type REQEvent struct {
	Authors []string
	Kinds   []int
}

type Tracker struct {
	mu             sync.RWMutex
	storage        *storage.Storage
	pubkeyRequests map[string]int64
	pubkeyByKind   map[string]map[int]int64
	cooccurrence   map[string]int64
	reqChan        chan REQEvent
	stopChan       chan struct{}
	flushInterval  time.Duration
}

func NewTracker(store *storage.Storage) *Tracker {
	return &Tracker{
		storage:        store,
		pubkeyRequests: make(map[string]int64),
		pubkeyByKind:   make(map[string]map[int]int64),
		cooccurrence:   make(map[string]int64),
		reqChan:        make(chan REQEvent, 10000),
		stopChan:       make(chan struct{}),
		flushInterval:  30 * time.Second,
	}
}

func (t *Tracker) Start(ctx context.Context) {
	go t.processLoop(ctx)
	go t.flushLoop(ctx)
}

func (t *Tracker) Stop() {
	close(t.stopChan)
}

func (t *Tracker) RecordREQ(filter nostr.Filter) {
	if len(filter.Authors) == 0 {
		return
	}

	select {
	case t.reqChan <- REQEvent{Authors: filter.Authors, Kinds: filter.Kinds}:
	default:
	}
}

func (t *Tracker) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopChan:
			return
		case evt := <-t.reqChan:
			t.processEvent(evt)
		}
	}
}

func (t *Tracker) processEvent(evt REQEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, pubkey := range evt.Authors {
		t.pubkeyRequests[pubkey]++

		if len(evt.Kinds) > 0 {
			if t.pubkeyByKind[pubkey] == nil {
				t.pubkeyByKind[pubkey] = make(map[int]int64)
			}
			for _, kind := range evt.Kinds {
				t.pubkeyByKind[pubkey][kind]++
			}
		}
	}

	// Track co-occurrences, but limit to first 20 authors to avoid O(n²) explosion
	maxPairAuthors := 20
	authorsForPairs := evt.Authors
	if len(authorsForPairs) > maxPairAuthors {
		authorsForPairs = authorsForPairs[:maxPairAuthors]
	}
	if len(authorsForPairs) >= 2 {
		for i := 0; i < len(authorsForPairs); i++ {
			for j := i + 1; j < len(authorsForPairs); j++ {
				key := makePairKey(authorsForPairs[i], authorsForPairs[j])
				t.cooccurrence[key]++
			}
		}
	}
}

func makePairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + ":" + b
}

func (t *Tracker) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(t.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.flush(context.Background())
			return
		case <-t.stopChan:
			t.flush(context.Background())
			return
		case <-ticker.C:
			t.flush(ctx)
		}
	}
}

func (t *Tracker) flush(ctx context.Context) {
	t.mu.Lock()
	pubkeyRequests := t.pubkeyRequests
	pubkeyByKind := t.pubkeyByKind
	cooccurrence := t.cooccurrence

	t.pubkeyRequests = make(map[string]int64)
	t.pubkeyByKind = make(map[string]map[int]int64)
	t.cooccurrence = make(map[string]int64)
	t.mu.Unlock()

	if len(pubkeyRequests) == 0 && len(cooccurrence) == 0 {
		return
	}

	err := t.storage.FlushREQAnalytics(ctx, pubkeyRequests, pubkeyByKind, cooccurrence)
	if err != nil {
		log.Printf("analytics: failed to flush REQ stats: %v", err)
	}
}

func (t *Tracker) GetPubkeyStats(ctx context.Context, pubkey string) (*storage.PubkeyStats, error) {
	return t.storage.GetPubkeyAnalytics(ctx, pubkey)
}

func (t *Tracker) GetTopRequested(ctx context.Context, limit int) ([]storage.PubkeyStats, error) {
	return t.storage.GetTopRequestedPubkeys(ctx, limit)
}

func (t *Tracker) GetTopCooccurring(ctx context.Context, limit int) ([]storage.CooccurrencePair, error) {
	return t.storage.GetTopCooccurrences(ctx, limit)
}
