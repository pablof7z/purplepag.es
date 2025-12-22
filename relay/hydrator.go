package relay

import (
	"context"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/purplepages/relay/storage"
)

type ProfileHydrator struct {
	storage         *storage.Storage
	relays          []string
	minFollowers    int
	retryAfterHours int
	batchSize       int
	stopChan        chan struct{}
}

func NewProfileHydrator(
	storage *storage.Storage,
	relays []string,
	minFollowers int,
	retryAfterHours int,
	batchSize int,
) *ProfileHydrator {
	return &ProfileHydrator{
		storage:         storage,
		relays:          relays,
		minFollowers:    minFollowers,
		retryAfterHours: retryAfterHours,
		batchSize:       batchSize,
		stopChan:        make(chan struct{}),
	}
}

func (h *ProfileHydrator) Start(ctx context.Context, intervalMinutes int) {
	ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)
	defer ticker.Stop()

	log.Printf("Profile hydrator started (min_followers=%d, retry_after=%dh, interval=%dm)",
		h.minFollowers, h.retryAfterHours, intervalMinutes)

	// Run immediately on start
	h.hydrate(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("Profile hydrator stopped")
			return
		case <-h.stopChan:
			log.Println("Profile hydrator stopped")
			return
		case <-ticker.C:
			h.hydrate(ctx)
		}
	}
}

func (h *ProfileHydrator) Stop() {
	close(h.stopChan)
}

func (h *ProfileHydrator) RunOnce(ctx context.Context) {
	h.hydrate(ctx)
}

func (h *ProfileHydrator) FindPubkeysNeedingHydration(ctx context.Context) []PubkeyNeed {
	return h.findPubkeysNeedingHydration(ctx)
}

func (h *ProfileHydrator) hydrate(ctx context.Context) {
	pubkeysToFetch := h.findPubkeysNeedingHydration(ctx)
	if len(pubkeysToFetch) == 0 {
		return
	}

	log.Printf("Profile hydrator: found %d pubkeys needing hydration", len(pubkeysToFetch))

	// Limit to batch size
	if len(pubkeysToFetch) > h.batchSize {
		pubkeysToFetch = pubkeysToFetch[:h.batchSize]
	}

	h.fetchProfiles(ctx, pubkeysToFetch)
}

type PubkeyNeed struct {
	Pubkey        string
	NeedKind0     bool
	NeedKind3     bool
	NeedKind10002 bool
}

func (h *ProfileHydrator) findPubkeysNeedingHydration(ctx context.Context) []PubkeyNeed {
	// Use optimized SQL query to count followers (much faster than loading all events)
	followerCounts, err := h.storage.GetFollowerCounts(ctx, h.minFollowers)
	if err != nil {
		log.Printf("Profile hydrator: failed to get follower counts: %v", err)
		return nil
	}

	retryThreshold := time.Now().Add(-time.Duration(h.retryAfterHours) * time.Hour).Unix()

	var needs []PubkeyNeed

	for pubkey, count := range followerCounts {
		if count < h.minFollowers {
			continue
		}

		// Check if we've already attempted recently
		attempt, _ := h.storage.GetProfileFetchAttempt(ctx, pubkey)
		if attempt != nil && attempt.LastAttempt > retryThreshold {
			// Already attempted recently, skip unless we're missing something
			if attempt.FetchedKind0 && attempt.FetchedKind3 && attempt.FetchedKind10002 {
				continue
			}
		}

		// Check what we currently have
		need := PubkeyNeed{Pubkey: pubkey}

		// Check kind 0
		k0Events, _ := h.storage.QueryEvents(ctx, nostr.Filter{
			Kinds:   []int{0},
			Authors: []string{pubkey},
			Limit:   1,
		})
		need.NeedKind0 = len(k0Events) == 0

		// Check kind 3
		k3Events, _ := h.storage.QueryEvents(ctx, nostr.Filter{
			Kinds:   []int{3},
			Authors: []string{pubkey},
			Limit:   1,
		})
		need.NeedKind3 = len(k3Events) == 0

		// Check kind 10002
		k10002Events, _ := h.storage.QueryEvents(ctx, nostr.Filter{
			Kinds:   []int{10002},
			Authors: []string{pubkey},
			Limit:   1,
		})
		need.NeedKind10002 = len(k10002Events) == 0

		// If we need any of them, add to list
		if need.NeedKind0 || need.NeedKind3 || need.NeedKind10002 {
			needs = append(needs, need)
		}
	}

	return needs
}

func (h *ProfileHydrator) fetchProfiles(ctx context.Context, needs []PubkeyNeed) {
	if len(h.relays) == 0 {
		log.Println("Profile hydrator: no relays configured for fetching")
		return
	}

	for _, relayURL := range h.relays {
		relay, err := nostr.RelayConnect(ctx, relayURL)
		if err != nil {
			log.Printf("Profile hydrator: failed to connect to %s: %v", relayURL, err)
			continue
		}

		h.fetchFromRelay(ctx, relay, needs)
		relay.Close()
	}
}

func (h *ProfileHydrator) fetchFromRelay(ctx context.Context, relay *nostr.Relay, needs []PubkeyNeed) {
	for _, need := range needs {
		var kinds []int
		if need.NeedKind0 {
			kinds = append(kinds, 0)
		}
		if need.NeedKind3 {
			kinds = append(kinds, 3)
		}
		if need.NeedKind10002 {
			kinds = append(kinds, 10002)
		}

		if len(kinds) == 0 {
			continue
		}

		filter := nostr.Filter{
			Kinds:   kinds,
			Authors: []string{need.Pubkey},
		}

		sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
		if err != nil {
			log.Printf("Profile hydrator: failed to subscribe for %s: %v", need.Pubkey[:16], err)
			continue
		}

		timeout := time.After(5 * time.Second)
		fetchedK0, fetchedK3, fetchedK10002 := false, false, false

	eventLoop:
		for {
			select {
			case <-ctx.Done():
				sub.Unsub()
				return
			case <-timeout:
				break eventLoop
			case evt := <-sub.Events:
				if evt == nil {
					continue
				}

				if err := h.storage.SaveEvent(ctx, evt); err != nil {
					if err.Error() != "duplicate: event already exists" {
						log.Printf("Profile hydrator: failed to save event: %v", err)
					}
				}

				switch evt.Kind {
				case 0:
					fetchedK0 = true
				case 3:
					fetchedK3 = true
				case 10002:
					fetchedK10002 = true
				}
			case <-sub.EndOfStoredEvents:
				break eventLoop
			}
		}

		sub.Unsub()

		// Record what we fetched (or that we tried)
		if err := h.storage.RecordProfileFetchAttempt(ctx, need.Pubkey, fetchedK0, fetchedK3, fetchedK10002); err != nil {
			log.Printf("Profile hydrator: failed to record attempt for %s: %v", need.Pubkey[:16], err)
		}

		if fetchedK0 || fetchedK3 || fetchedK10002 {
			log.Printf("Profile hydrator: fetched data for %s (k0=%t, k3=%t, k10002=%t)",
				need.Pubkey[:16], fetchedK0, fetchedK3, fetchedK10002)
		}
	}
}
