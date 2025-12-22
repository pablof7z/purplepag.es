package relay

import (
	"context"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/storage"
)

type SyncQueue struct {
	storage      *storage.Storage
	allowedKinds []int
	stopChan     chan struct{}
}

func NewSyncQueue(storage *storage.Storage, allowedKinds []int) *SyncQueue {
	return &SyncQueue{
		storage:      storage,
		allowedKinds: allowedKinds,
		stopChan:     make(chan struct{}),
	}
}

func (sq *SyncQueue) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("Relay sync queue started")

	for {
		select {
		case <-ctx.Done():
			log.Println("Relay sync queue stopped")
			return
		case <-sq.stopChan:
			log.Println("Relay sync queue stopped")
			return
		case <-ticker.C:
			sq.processNextRelay(ctx)
		}
	}
}

func (sq *SyncQueue) Stop() {
	close(sq.stopChan)
}

func (sq *SyncQueue) processNextRelay(ctx context.Context) {
	relays, err := sq.storage.GetRelayQueue(ctx)
	if err != nil {
		log.Printf("Failed to get relay queue: %v", err)
		return
	}

	if len(relays) == 0 {
		return
	}

	relay := relays[0]

	log.Printf("Syncing with %s...", relay.URL)

	eventsContributed, err := sq.syncRelay(ctx, relay.URL)
	if err != nil {
		log.Printf("Failed to sync with %s: %v", relay.URL, err)
		if err := sq.storage.UpdateSyncStats(ctx, relay.URL, false, 0); err != nil {
			log.Printf("Failed to update sync stats for %s: %v", relay.URL, err)
		}
		return
	}

	log.Printf("Synced with %s, contributed %d new events", relay.URL, eventsContributed)

	if err := sq.storage.UpdateSyncStats(ctx, relay.URL, true, int64(eventsContributed)); err != nil {
		log.Printf("Failed to update sync stats for %s: %v", relay.URL, err)
	}
}

func (sq *SyncQueue) syncRelay(ctx context.Context, relayURL string) (int, error) {
	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return 0, err
	}
	defer relay.Close()

	totalNewEvents := 0

	for _, kind := range sq.allowedKinds {
		newEvents, err := sq.syncKind(ctx, relay, kind)
		if err != nil {
			log.Printf("Failed to sync kind %d from %s: %v", kind, relayURL, err)
			continue
		}
		totalNewEvents += newEvents
	}

	return totalNewEvents, nil
}

func (sq *SyncQueue) syncKind(ctx context.Context, relay *nostr.Relay, kind int) (int, error) {
	filter := nostr.Filter{
		Kinds: []int{kind},
		Limit: 500,
	}

	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		return 0, err
	}
	defer sub.Unsub()

	timeout := time.After(10 * time.Second)
	newEventCount := 0

	for {
		select {
		case <-ctx.Done():
			return newEventCount, ctx.Err()
		case <-timeout:
			return newEventCount, nil
		case evt := <-sub.Events:
			if evt == nil {
				continue
			}

			if err := sq.storage.SaveEvent(ctx, evt); err != nil {
				if err.Error() == "duplicate: event already exists" {
					continue
				}
				log.Printf("Failed to save event %s: %v", evt.ID, err)
				continue
			}
			newEventCount++

		case <-sub.EndOfStoredEvents:
			return newEventCount, nil
		}
	}
}
