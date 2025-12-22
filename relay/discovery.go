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
