package relay

import (
	"context"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/purplepages/relay/analytics"
	"github.com/purplepages/relay/storage"
)

type TrustedSyncer struct {
	storage       *storage.Storage
	trustAnalyzer *analytics.TrustAnalyzer
	kinds         []int
	batchSize     int
	timeout       time.Duration
	stopChan      chan struct{}
}

func NewTrustedSyncer(
	storage *storage.Storage,
	trustAnalyzer *analytics.TrustAnalyzer,
	kinds []int,
	batchSize int,
	timeoutSeconds int,
) *TrustedSyncer {
	return &TrustedSyncer{
		storage:       storage,
		trustAnalyzer: trustAnalyzer,
		kinds:         kinds,
		batchSize:     batchSize,
		timeout:       time.Duration(timeoutSeconds) * time.Second,
		stopChan:      make(chan struct{}),
	}
}

func (s *TrustedSyncer) Start(ctx context.Context, intervalMinutes int) {
	ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)
	defer ticker.Stop()

	log.Printf("Trusted syncer started (batch_size=%d, interval=%dm, timeout=%s, kinds=%v)",
		s.batchSize, intervalMinutes, s.timeout, s.kinds)

	// Run immediately on start
	s.sync(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("Trusted syncer stopped")
			return
		case <-s.stopChan:
			log.Println("Trusted syncer stopped")
			return
		case <-ticker.C:
			s.sync(ctx)
		}
	}
}

func (s *TrustedSyncer) Stop() {
	close(s.stopChan)
}

func (s *TrustedSyncer) sync(ctx context.Context) {
	// Get all trusted pubkeys
	trustedPubkeys := s.trustAnalyzer.GetTrustedPubkeys()
	if len(trustedPubkeys) == 0 {
		log.Println("Trusted syncer: no trusted pubkeys available yet")
		return
	}

	// Get batch ordered by oldest sync time
	queue, err := s.storage.GetTrustedSyncQueue(ctx, trustedPubkeys, s.batchSize)
	if err != nil {
		log.Printf("Trusted syncer: failed to get sync queue: %v", err)
		return
	}

	if len(queue) == 0 {
		return
	}

	log.Printf("Trusted syncer: syncing %d pubkeys (of %d trusted)", len(queue), len(trustedPubkeys))

	for _, state := range queue {
		s.syncPubkey(ctx, state.Pubkey, state.LastSyncedAt)
	}
}

func (s *TrustedSyncer) syncPubkey(ctx context.Context, pubkey string, lastSyncedAt int64) {
	// Get write relays from pubkey's 10002
	relayURLs, err := s.storage.GetPubkeyRelayList(ctx, pubkey)
	if err != nil {
		log.Printf("Trusted syncer: failed to get relay list for %s: %v", pubkey[:16], err)
		return
	}

	if len(relayURLs) == 0 {
		// No 10002 event, skip this pubkey but still mark as synced to avoid retrying constantly
		if err := s.storage.UpdateTrustedSyncState(ctx, pubkey); err != nil {
			log.Printf("Trusted syncer: failed to update sync state for %s: %v", pubkey[:16], err)
		}
		return
	}

	// Build filter for events since last sync
	filter := nostr.Filter{
		Kinds:   s.kinds,
		Authors: []string{pubkey},
	}
	if lastSyncedAt > 0 {
		since := nostr.Timestamp(lastSyncedAt)
		filter.Since = &since
	}

	eventsFound := 0

	// Try each write relay
	for _, relayURL := range relayURLs {
		normalized, err := NormalizeRelayURL(relayURL)
		if err != nil {
			continue
		}

		count := s.fetchFromRelay(ctx, normalized, filter)
		eventsFound += count
	}

	// Update sync state regardless of success (best effort approach)
	if err := s.storage.UpdateTrustedSyncState(ctx, pubkey); err != nil {
		log.Printf("Trusted syncer: failed to update sync state for %s: %v", pubkey[:16], err)
	}

	if eventsFound > 0 {
		log.Printf("Trusted syncer: fetched %d events for %s from %d relays",
			eventsFound, pubkey[:16], len(relayURLs))
	}
}

func (s *TrustedSyncer) fetchFromRelay(ctx context.Context, relayURL string, filter nostr.Filter) int {
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	relay, err := nostr.RelayConnect(timeoutCtx, relayURL)
	if err != nil {
		return 0
	}
	defer relay.Close()

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
					log.Printf("Trusted syncer: failed to save event: %v", err)
				}
			} else {
				count++
			}
		case <-sub.EndOfStoredEvents:
			return count
		}
	}
}
