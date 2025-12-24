package relay

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/storage"
)

type CrossKindSyncer struct {
	storage    *storage.Storage
	relays     []string
	kinds      []int
	batchSize  int
	batchDelay time.Duration
	timeout    time.Duration
	stopChan   chan struct{}
}

func NewCrossKindSyncer(
	storage *storage.Storage,
	relays []string,
	kinds []int,
	batchSize int,
	batchDelayMs int,
	timeoutSeconds int,
) *CrossKindSyncer {
	return &CrossKindSyncer{
		storage:    storage,
		relays:     relays,
		kinds:      kinds,
		batchSize:  batchSize,
		batchDelay: time.Duration(batchDelayMs) * time.Millisecond,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
		stopChan:   make(chan struct{}),
	}
}

func (s *CrossKindSyncer) RunOnce(ctx context.Context) {
	if len(s.kinds) < 2 {
		log.Println("Cross-kind syncer: need at least 2 sync kinds to operate")
		return
	}

	log.Printf("Cross-kind syncer: starting for kinds %v", s.kinds)

	// Step 1: Get event counts by kind
	counts, err := s.storage.GetEventCountsByKind(ctx)
	if err != nil {
		log.Printf("Cross-kind syncer: failed to get event counts: %v", err)
		return
	}

	// Step 2: Filter to sync kinds and find source (highest count)
	type kindCount struct {
		kind  int
		count int64
	}
	var kindCounts []kindCount
	for _, k := range s.kinds {
		kindCounts = append(kindCounts, kindCount{kind: k, count: counts[k]})
	}

	// Sort by count descending to find source
	sort.Slice(kindCounts, func(i, j int) bool {
		return kindCounts[i].count > kindCounts[j].count
	})

	sourceKind := kindCounts[0].kind
	sourceCount := kindCounts[0].count

	log.Printf("Cross-kind syncer: source kind=%d (%d events)", sourceKind, sourceCount)

	// Step 3: For each target kind (sorted by ascending count - smallest gaps first)
	for i := len(kindCounts) - 1; i >= 0; i-- {
		kc := kindCounts[i]
		if kc.kind == sourceKind {
			continue
		}

		select {
		case <-ctx.Done():
			log.Println("Cross-kind syncer: cancelled")
			return
		case <-s.stopChan:
			log.Println("Cross-kind syncer: stopped")
			return
		default:
		}

		gap := sourceCount - kc.count
		log.Printf("Cross-kind syncer: syncing kind=%d (%d events, gap=%d)", kc.kind, kc.count, gap)
		s.syncMissingKind(ctx, sourceKind, kc.kind)
	}

	log.Println("Cross-kind syncer: completed")
}

func (s *CrossKindSyncer) syncMissingKind(ctx context.Context, sourceKind, targetKind int) {
	totalProcessed := 0
	totalHits := 0
	batchNum := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		// Get batch of pubkeys with sourceKind but not targetKind
		pubkeys, err := s.storage.GetPubkeysMissingKind(ctx, sourceKind, targetKind, s.batchSize)
		if err != nil {
			log.Printf("Cross-kind syncer: query failed: %v", err)
			return
		}

		if len(pubkeys) == 0 {
			log.Printf("Cross-kind syncer: kind=%d complete - processed=%d, hits=%d, misses=%d",
				targetKind, totalProcessed, totalHits, totalProcessed-totalHits)
			return
		}

		batchNum++
		log.Printf("Cross-kind syncer: batch %d - fetching kind:%d for %d pubkeys",
			batchNum, targetKind, len(pubkeys))

		// Fetch from each relay
		batchHits := 0
		for _, relayURL := range s.relays {
			hits := s.fetchFromRelay(ctx, relayURL, pubkeys, targetKind)
			batchHits += hits
		}

		totalProcessed += len(pubkeys)
		totalHits += batchHits

		log.Printf("Cross-kind syncer: batch %d complete - hits=%d, total_processed=%d",
			batchNum, batchHits, totalProcessed)

		// Delay between batches to avoid rate limiting
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-time.After(s.batchDelay):
		}
	}
}

func (s *CrossKindSyncer) fetchFromRelay(ctx context.Context, relayURL string, pubkeys []string, kind int) int {
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	relay, err := nostr.RelayConnect(timeoutCtx, relayURL)
	if err != nil {
		return 0
	}
	defer relay.Close()

	filter := nostr.Filter{
		Kinds:   []int{kind},
		Authors: pubkeys,
	}

	sub, err := relay.Subscribe(timeoutCtx, []nostr.Filter{filter})
	if err != nil {
		return 0
	}
	defer sub.Unsub()

	count := 0
	for {
		select {
		case <-timeoutCtx.Done():
			return count
		case evt := <-sub.Events:
			if evt == nil {
				continue
			}
			if err := s.storage.SaveEvent(ctx, evt); err != nil {
				if err.Error() != "duplicate: event already exists" {
					log.Printf("Cross-kind syncer: failed to save event: %v", err)
				}
			} else {
				count++
			}
		case <-sub.EndOfStoredEvents:
			return count
		}
	}
}

func (s *CrossKindSyncer) Stop() {
	close(s.stopChan)
}
