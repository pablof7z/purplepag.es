package storage

import (
	"context"
	"encoding/hex"
	"time"

	"fiatjaf.com/nostr/nip45/hyperloglog"
	"github.com/fiatjaf/eventstore"
	"github.com/nbd-wtf/go-nostr"
)

// CountEvents implements khatru's COUNT handler interface
// This is called by khatru when it receives a COUNT message from a client
func (s *Storage) CountEvents(ctx context.Context, filter nostr.Filter) (int64, error) {
	// Check if the backend implements the Counter interface
	if counter, ok := s.db.(eventstore.Counter); ok {
		return counter.CountEvents(ctx, filter)
	}

	// Fallback: query and count
	events, err := s.QueryEvents(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int64(len(events)), nil
}

// CountEventsHLL queries matching events and computes a HyperLogLog from their pubkeys.
// This streams events from the backend to avoid loading them all into memory.
func (s *Storage) CountEventsHLL(ctx context.Context, filter nostr.Filter, offset int) (uint32, *hyperloglog.HyperLogLog, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ch, err := s.db.QueryEvents(ctx, filter)
	if err != nil {
		return 0, nil, err
	}

	hll := hyperloglog.New(offset)
	var count uint32
	for evt := range ch {
		var pk [32]byte
		hex.Decode(pk[:], []byte(evt.PubKey))
		hll.Add(pk)
		count++
	}

	return count, hll, nil
}
