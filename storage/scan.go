package storage

import (
	"context"
	"fmt"

	"github.com/fiatjaf/eventstore"
	"github.com/nbd-wtf/go-nostr"
)

const defaultScanPageSize = 100000

const maxUint32 = ^uint32(0)

// ScanEvents iterates over all events that match the filter, in descending time order.
// It uses time-based pagination and de-duplicates when a single timestamp spans pages.
func (s *Storage) ScanEvents(ctx context.Context, filter nostr.Filter, pageSize int, onEvent func(*nostr.Event) error) error {
	if onEvent == nil {
		return fmt.Errorf("scan events: onEvent callback is nil")
	}
	if pageSize <= 0 {
		pageSize = defaultScanPageSize
	}

	baseFilter := filter
	baseFilter.Limit = 0
	baseFilter.Since = nil
	baseFilter.Until = nil

	startSince := uint32(0)
	if filter.Since != nil {
		startSince = uint32(*filter.Since)
	}
	startUntil := uint32(maxUint32)
	if filter.Until != nil {
		startUntil = uint32(*filter.Until)
	}
	if startSince > startUntil {
		return nil
	}

	ctx = eventstore.SetNegentropy(ctx)

	since := nostr.Timestamp(startSince)
	until := nostr.Timestamp(startUntil)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		queryFilter := baseFilter
		queryFilter.Since = &since
		queryFilter.Until = &until
		queryFilter.Limit = pageSize

		ch, err := s.db.QueryEvents(ctx, queryFilter)
		if err != nil {
			return err
		}

		events := make([]*nostr.Event, 0, pageSize)
		for evt := range ch {
			events = append(events, evt)
		}

		if len(events) == 0 {
			return nil
		}

		for _, evt := range events {
			if err := onEvent(evt); err != nil {
				return err
			}
		}

		if len(events) < pageSize {
			return nil
		}

		oldest := events[len(events)-1]
		if oldest == nil {
			return nil
		}
		oldestTs := uint32(oldest.CreatedAt)

		countAtOldest := 0
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].CreatedAt != oldest.CreatedAt {
				break
			}
			countAtOldest++
		}

		if countAtOldest > 0 {
			seen := make(map[string]struct{}, countAtOldest)
			for _, evt := range events[len(events)-countAtOldest:] {
				seen[evt.ID] = struct{}{}
			}

			exact := baseFilter
			timestamp := nostr.Timestamp(oldestTs)
			exact.Since = &timestamp
			exact.Until = &timestamp
			exact.Limit = 0

			ch, err := s.db.QueryEvents(ctx, exact)
			if err != nil {
				return err
			}
			for evt := range ch {
				if _, ok := seen[evt.ID]; ok {
					continue
				}
				if err := onEvent(evt); err != nil {
					return err
				}
			}
		}

		if oldest.CreatedAt <= since {
			return nil
		}
		if oldestTs == 0 {
			return nil
		}

		until = nostr.Timestamp(oldestTs - 1)
		if until < since {
			return nil
		}
	}
}
