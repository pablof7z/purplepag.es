package storage

import (
	"context"
	"math/rand"
	"sort"
	"time"

	"github.com/pablof7z/purplepag.es/internal/relayutil"
)

type DiscoveredRelay struct {
	URL         string
	PubkeyCount int64
}

func (s *Storage) GetRelayQueue(ctx context.Context) ([]DiscoveredRelay, error) {
	relays, err := s.GetRelayStats(ctx)
	if err != nil {
		return nil, err
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(relays), func(i, j int) {
		relays[i], relays[j] = relays[j], relays[i]
	})

	return relays, nil
}

func (s *Storage) UpdateSyncStats(ctx context.Context, url string, success bool, eventsContributed int64) error {
	return nil
}

func (s *Storage) GetRelayStats(ctx context.Context) ([]DiscoveredRelay, error) {
	if cached, err := s.GetCachedRelayStats(ctx); err == nil {
		return cached, nil
	}

	latest, err := s.latestEventsByPubkey(ctx, 10002)
	if err != nil {
		return nil, err
	}

	relayPubkeys := make(map[string]map[string]struct{})
	for pubkey, evt := range latest {
		for _, tag := range evt.Tags {
			if len(tag) < 2 || tag[0] != "r" {
				continue
			}
			normalized, err := relayutil.NormalizeRelayURL(tag[1])
			if err != nil {
				continue
			}
			if relayPubkeys[normalized] == nil {
				relayPubkeys[normalized] = make(map[string]struct{})
			}
			relayPubkeys[normalized][pubkey] = struct{}{}
		}
	}

	relays := make([]DiscoveredRelay, 0, len(relayPubkeys))
	for url, pubkeys := range relayPubkeys {
		relays = append(relays, DiscoveredRelay{
			URL:         url,
			PubkeyCount: int64(len(pubkeys)),
		})
	}

	sort.Slice(relays, func(i, j int) bool {
		if relays[i].PubkeyCount == relays[j].PubkeyCount {
			return relays[i].URL < relays[j].URL
		}
		return relays[i].PubkeyCount > relays[j].PubkeyCount
	})

	return relays, nil
}

func (s *Storage) GetDiscoveredRelayCount(ctx context.Context) (int64, error) {
	relays, err := s.GetRelayStats(ctx)
	if err != nil {
		return 0, err
	}
	return int64(len(relays)), nil
}

func (s *Storage) GetFollowerCounts(ctx context.Context, minFollowers int) (map[string]int, error) {
	if cached, err := s.GetCachedFollowerCounts(ctx, minFollowers); err == nil {
		return cached, nil
	}

	latest, err := s.latestEventsByPubkey(ctx, 3)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, evt := range latest {
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				counts[tag[1]]++
			}
		}
	}

	result := make(map[string]int)
	for pubkey, count := range counts {
		if count >= minFollowers {
			result[pubkey] = count
		}
	}

	return result, nil
}
