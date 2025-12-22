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

func (s *Syncer) syncKind(ctx context.Context, relay *nostr.Relay, kind int) error {
	log.Printf("Syncing kind %d from %s...", kind, relay.URL)

	filter := nostr.Filter{
		Kinds: []int{kind},
	}

	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		return err
	}
	defer sub.Unsub()

	eventCount := 0
	newEvents := 0

	// Idle timeout - resets each time we receive an event
	idleTimeout := 30 * time.Second
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			log.Printf("Sync idle timeout for kind %d: received %d events, saved %d new events", kind, eventCount, newEvents)
			return nil
		case evt := <-sub.Events:
			if evt == nil {
				continue
			}
			eventCount++

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

			if eventCount%1000 == 0 {
				log.Printf("Progress: received %d events, saved %d new (kind %d)", eventCount, newEvents, kind)
			}
		case <-sub.EndOfStoredEvents:
			log.Printf("Sync complete for kind %d: received %d events, saved %d new events", kind, eventCount, newEvents)
			return nil
		}
	}
}
