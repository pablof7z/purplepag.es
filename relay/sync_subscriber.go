package relay

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/storage"
)

type SyncSubscriber struct {
	storage  *storage.Storage
	relays   []string
	kinds    []int
	stopChan chan struct{}
	wg       sync.WaitGroup
}

func NewSyncSubscriber(storage *storage.Storage, relays []string, kinds []int) *SyncSubscriber {
	return &SyncSubscriber{
		storage:  storage,
		relays:   relays,
		kinds:    kinds,
		stopChan: make(chan struct{}),
	}
}

func (s *SyncSubscriber) Start(ctx context.Context) {
	log.Printf("Sync subscriber: starting persistent subscriptions to %d relays for kinds %v",
		len(s.relays), s.kinds)

	for _, relayURL := range s.relays {
		s.wg.Add(1)
		go s.subscribeToRelay(ctx, relayURL)
	}
}

func (s *SyncSubscriber) subscribeToRelay(ctx context.Context, relayURL string) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		s.connectAndSubscribe(ctx, relayURL)

		// Wait before reconnecting
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-time.After(30 * time.Second):
			log.Printf("Sync subscriber: reconnecting to %s", relayURL)
		}
	}
}

func (s *SyncSubscriber) connectAndSubscribe(ctx context.Context, relayURL string) {
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	relay, err := nostr.RelayConnect(connectCtx, relayURL)
	cancel()

	if err != nil {
		log.Printf("Sync subscriber: failed to connect to %s: %v", relayURL, err)
		return
	}
	defer relay.Close()

	// Subscribe with all sync kinds, no time filter (get everything new)
	now := nostr.Now()
	filter := nostr.Filter{
		Kinds: s.kinds,
		Since: &now,
	}

	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		log.Printf("Sync subscriber: failed to subscribe to %s: %v", relayURL, err)
		return
	}
	defer sub.Unsub()

	log.Printf("Sync subscriber: connected to %s, listening for kinds %v", relayURL, s.kinds)

	eventsReceived := 0
	for {
		select {
		case <-ctx.Done():
			if eventsReceived > 0 {
				log.Printf("Sync subscriber: %s - received %d events before shutdown", relayURL, eventsReceived)
			}
			return
		case <-s.stopChan:
			if eventsReceived > 0 {
				log.Printf("Sync subscriber: %s - received %d events before stop", relayURL, eventsReceived)
			}
			return
		case evt := <-sub.Events:
			if evt == nil {
				continue
			}
			if err := s.storage.SaveEvent(ctx, evt); err != nil {
				if err.Error() != "duplicate: event already exists" {
					log.Printf("Sync subscriber: failed to save event from %s: %v", relayURL, err)
				}
			} else {
				eventsReceived++
				if eventsReceived%100 == 0 {
					log.Printf("Sync subscriber: %s - received %d events", relayURL, eventsReceived)
				}
			}
		case <-relay.Context().Done():
			log.Printf("Sync subscriber: connection to %s closed (received %d events)", relayURL, eventsReceived)
			return
		}
	}
}

func (s *SyncSubscriber) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	log.Println("Sync subscriber: stopped")
}
