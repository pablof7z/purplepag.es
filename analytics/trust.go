package analytics

import (
	"context"
	"log"
	"sync"

	"github.com/pablof7z/purplepag.es/storage"
)

type TrustAnalyzer struct {
	mu                  sync.RWMutex
	storage             *storage.Storage
	clusterDetector     *ClusterDetector
	trustedSet          map[string]bool
	minTrustedFollowers int
}

func NewTrustAnalyzer(store *storage.Storage, clusterDetector *ClusterDetector, minTrustedFollowers int) *TrustAnalyzer {
	if minTrustedFollowers <= 0 {
		minTrustedFollowers = 10
	}
	t := &TrustAnalyzer{
		storage:             store,
		clusterDetector:     clusterDetector,
		trustedSet:          make(map[string]bool),
		minTrustedFollowers: minTrustedFollowers,
	}

	// Load trusted pubkeys from database on startup
	ctx := context.Background()
	pubkeys, err := store.GetTrustedPubkeys(ctx)
	if err != nil {
		log.Printf("analytics: failed to load trusted pubkeys from database: %v", err)
	} else if len(pubkeys) > 0 {
		for _, pk := range pubkeys {
			t.trustedSet[pk] = true
		}
		log.Printf("analytics: loaded %d trusted pubkeys from database", len(pubkeys))
	}

	return t
}

func (t *TrustAnalyzer) AnalyzeTrust(ctx context.Context) error {
	graph := t.clusterDetector.GetFollowGraph(ctx)
	return t.AnalyzeTrustWithGraph(ctx, graph)
}

func (t *TrustAnalyzer) AnalyzeTrustWithGraph(ctx context.Context, graph FollowGraph) error {
	log.Println("analytics: starting trust analysis")

	if err := t.storage.ClearSpamCandidates(ctx); err != nil {
		log.Printf("analytics: failed to clear spam candidates: %v", err)
	}

	if len(graph) == 0 {
		log.Println("analytics: no follow graph data available")
		return nil
	}

	trusted := t.findLargestConnectedComponent(graph)
	log.Printf("analytics: seed trusted set from largest component: %d pubkeys", len(trusted))

	// Trust propagation via follow graph:
	// A pubkey becomes trusted if >= minTrustedFollowers trusted users follow it
	changed := true
	iterations := 0
	for changed && iterations < 100 {
		changed = false
		iterations++

		// Count trusted followers for each non-trusted pubkey
		trustedFollowerCount := make(map[string]int)
		for follower, following := range graph {
			if !trusted[follower] {
				continue
			}
			for followed := range following {
				if !trusted[followed] {
					trustedFollowerCount[followed]++
				}
			}
		}

		// Promote pubkeys with enough trusted followers
		for pubkey, count := range trustedFollowerCount {
			if count >= t.minTrustedFollowers {
				trusted[pubkey] = true
				changed = true
			}
		}
	}

	log.Printf("analytics: trust propagation complete after %d iterations, %d trusted pubkeys", iterations, len(trusted))

	t.mu.Lock()
	t.trustedSet = trusted
	t.mu.Unlock()

	// Persist trusted pubkeys to database for use by other components (e.g., event archiving)
	trustedList := make([]string, 0, len(trusted))
	for pk := range trusted {
		trustedList = append(trustedList, pk)
	}
	if err := t.storage.SetTrustedPubkeys(ctx, trustedList); err != nil {
		log.Printf("analytics: failed to persist trusted pubkeys: %v", err)
	}

	clusters, err := t.storage.GetBotClusters(ctx, 1000)
	if err != nil {
		log.Printf("analytics: failed to get bot clusters: %v", err)
		return err
	}

	spamCount := 0
	for _, cluster := range clusters {
		for _, pubkey := range cluster.Members {
			if !trusted[pubkey] {
				eventCount, _ := t.storage.CountEventsForPubkey(ctx, pubkey)
				if eventCount > 0 {
					err := t.storage.SaveSpamCandidate(ctx, pubkey, "isolated_cluster", eventCount)
					if err != nil {
						log.Printf("analytics: failed to save spam candidate: %v", err)
					}
					spamCount++
				}
			}
		}
	}

	// Check for pubkeys that have events but were never requested
	reqData, err := t.storage.GetAllRequestedPubkeys(ctx)
	if err != nil {
		log.Printf("analytics: failed to get REQ data: %v", err)
		reqData = make(map[string]int64)
	}

	allPubkeys := t.getAllPubkeysWithEvents(graph)
	for pubkey := range allPubkeys {
		if trusted[pubkey] {
			continue
		}

		if reqData[pubkey] == 0 {
			eventCount, _ := t.storage.CountEventsForPubkey(ctx, pubkey)
			if eventCount > 0 {
				err := t.storage.SaveSpamCandidate(ctx, pubkey, "never_requested", eventCount)
				if err != nil {
					log.Printf("analytics: failed to save spam candidate: %v", err)
				}
				spamCount++
			}
		}
	}

	log.Printf("analytics: identified %d spam candidates", spamCount)

	return nil
}

func (t *TrustAnalyzer) findLargestConnectedComponent(graph FollowGraph) map[string]bool {
	allNodes := make(map[string]bool)
	for node := range graph {
		allNodes[node] = true
		for followed := range graph[node] {
			allNodes[followed] = true
		}
	}

	undirected := make(map[string]map[string]bool)
	for node := range allNodes {
		undirected[node] = make(map[string]bool)
	}
	for from, tos := range graph {
		for to := range tos {
			undirected[from][to] = true
			undirected[to][from] = true
		}
	}

	visited := make(map[string]bool)
	var largestComponent map[string]bool

	for node := range allNodes {
		if visited[node] {
			continue
		}

		component := make(map[string]bool)
		queue := []string{node}

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			if visited[current] {
				continue
			}
			visited[current] = true
			component[current] = true

			for neighbor := range undirected[current] {
				if !visited[neighbor] {
					queue = append(queue, neighbor)
				}
			}
		}

		if len(component) > len(largestComponent) {
			largestComponent = component
		}
	}

	return largestComponent
}

func (t *TrustAnalyzer) getAllPubkeysWithEvents(graph FollowGraph) map[string]bool {
	pubkeys := make(map[string]bool)
	for pubkey := range graph {
		pubkeys[pubkey] = true
	}
	return pubkeys
}

func (t *TrustAnalyzer) IsTrusted(pubkey string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.trustedSet[pubkey]
}

func (t *TrustAnalyzer) GetTrustedCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.trustedSet)
}

func (t *TrustAnalyzer) GetTrustedPubkeys() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	pubkeys := make([]string, 0, len(t.trustedSet))
	for pk := range t.trustedSet {
		pubkeys = append(pubkeys, pk)
	}
	return pubkeys
}

func (t *TrustAnalyzer) GetSpamCandidates(ctx context.Context, limit int) ([]storage.SpamCandidate, error) {
	return t.storage.GetSpamCandidates(ctx, limit)
}

// GetTrustedFollowerCount returns how many trusted pubkeys follow the given pubkey
func (t *TrustAnalyzer) GetTrustedFollowerCount(ctx context.Context, pubkey string) (int, error) {
	followers, err := t.storage.GetFollowersOfPubkey(ctx, pubkey)
	if err != nil {
		return 0, err
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, follower := range followers {
		if t.trustedSet[follower] {
			count++
		}
	}

	return count, nil
}
