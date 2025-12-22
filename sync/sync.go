package sync

import (
	"context"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/storage"
)

type Syncer struct {
	storage      *storage.Storage
	allowedKinds []int
	relays       []string
}

func NewSyncer(storage *storage.Storage, allowedKinds []int, relays []string) *Syncer {
	return &Syncer{
		storage:      storage,
		allowedKinds: allowedKinds,
		relays:       relays,
	}
}

func (s *Syncer) SyncAll(ctx context.Context) error {
	for _, relayURL := range s.relays {
		if err := s.syncRelay(ctx, relayURL); err != nil {
			log.Printf("Failed to sync from %s: %v", relayURL, err)
			continue
		}
		log.Printf("Successfully synced from %s", relayURL)
	}
	return nil
}

func (s *Syncer) syncRelay(ctx context.Context, relayURL string) error {
	log.Printf("Connecting to %s for sync...", relayURL)
	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return err
	}
	defer relay.Close()
	log.Printf("Connected to %s", relayURL)

	for _, kind := range s.allowedKinds {
		if err := s.syncKind(ctx, relay, kind); err != nil {
			log.Printf("Failed to sync kind %d from %s: %v", kind, relayURL, err)
		}
	}

	return nil
}

const syncLimit = 500

func (s *Syncer) syncKind(ctx context.Context, relay *nostr.Relay, kind int) error {
	log.Printf("Syncing kind %d from %s...", kind, relay.URL)

	totalEvents := 0
	totalNew := 0
	var until *nostr.Timestamp

	for {
		batchEvents, batchNew, oldestTime, err := s.syncKindBatch(ctx, relay, kind, until)
		if err != nil {
			return err
		}

		totalEvents += batchEvents
		totalNew += batchNew

		// If we got less than the limit, we've fetched everything
		if batchEvents < syncLimit {
			log.Printf("Sync complete for kind %d: received %d events, saved %d new events", kind, totalEvents, totalNew)
			return nil
		}

		// Continue from before the oldest event we received
		if oldestTime != nil {
			t := nostr.Timestamp(*oldestTime - 1)
			until = &t
			log.Printf("Continuing sync for kind %d (fetched %d so far, until %d)...", kind, totalEvents, *until)
		} else {
			// No events received, we're done
			log.Printf("Sync complete for kind %d: received %d events, saved %d new events", kind, totalEvents, totalNew)
			return nil
		}
	}
}

func (s *Syncer) syncKindBatch(ctx context.Context, relay *nostr.Relay, kind int, until *nostr.Timestamp) (eventCount int, newEvents int, oldestTime *nostr.Timestamp, err error) {
	filter := nostr.Filter{
		Kinds: []int{kind},
		Limit: syncLimit,
		Until: until,
	}

	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		return 0, 0, nil, err
	}
	defer sub.Unsub()

	// Idle timeout - resets each time we receive an event
	idleTimeout := 30 * time.Second
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return eventCount, newEvents, oldestTime, ctx.Err()
		case <-timer.C:
			return eventCount, newEvents, oldestTime, nil
		case evt := <-sub.Events:
			if evt == nil {
				continue
			}
			eventCount++

			// Track oldest event for pagination
			if oldestTime == nil || evt.CreatedAt < *oldestTime {
				oldestTime = &evt.CreatedAt
			}

			// Reset idle timer
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)

			if err := s.storage.SaveEvent(ctx, evt); err == nil {
				newEvents++
			}
		case <-sub.EndOfStoredEvents:
			return eventCount, newEvents, oldestTime, nil
		}
	}
}
