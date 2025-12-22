package relay

import (
	"context"
	"log"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/storage"
)

type Discovery struct {
	storage   *storage.Storage
	newRelays chan string
}

func NewDiscovery(storage *storage.Storage) *Discovery {
	return &Discovery{
		storage:   storage,
		newRelays: make(chan string, 1000),
	}
}

func (d *Discovery) ExtractRelaysFromEvent(ctx context.Context, evt *nostr.Event) {
	if evt.Kind != 10002 {
		return
	}

	for _, tag := range evt.Tags {
		if len(tag) < 2 {
			continue
		}

		if tag[0] != "r" {
			continue
		}

		rawURL := tag[1]

		normalized, err := NormalizeRelayURL(rawURL)
		if err != nil {
			continue
		}

		if err := d.storage.AddDiscoveredRelay(ctx, normalized); err != nil {
			log.Printf("Failed to add discovered relay %s: %v", normalized, err)
			continue
		}

		select {
		case d.newRelays <- normalized:
		default:
		}
	}
}

func (d *Discovery) NewRelaysChan() <-chan string {
	return d.newRelays
}

// BackfillDiscoveredRelays extracts relay URLs from all existing kind 10002 events
// and adds them to the discovered_relays table (normalized)
func (d *Discovery) BackfillDiscoveredRelays(ctx context.Context) error {
	log.Println("Backfilling discovered relays from existing events...")

	rawURLs, err := d.storage.GetRawRelayURLsFromEvents(ctx)
	if err != nil {
		return err
	}

	log.Printf("Found %d distinct raw relay URLs to process", len(rawURLs))

	added := 0
	for _, rawURL := range rawURLs {
		normalized, err := NormalizeRelayURL(rawURL)
		if err != nil {
			continue
		}

		if err := d.storage.AddDiscoveredRelay(ctx, normalized); err != nil {
			log.Printf("Failed to add discovered relay %s: %v", normalized, err)
			continue
		}
		added++
	}

	log.Printf("Backfill complete: added %d normalized relay URLs", added)
	return nil
}
