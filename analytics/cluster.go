package analytics

import (
	"context"
	"log"

	"github.com/nbd-wtf/go-nostr"
	"github.com/purplepages/relay/storage"
)

type ClusterDetector struct {
	storage          *storage.Storage
	minClusterSize   int
	minDensity       float64
	maxExternalRatio float64
}

func NewClusterDetector(store *storage.Storage) *ClusterDetector {
	return &ClusterDetector{
		storage:          store,
		minClusterSize:   5,
		minDensity:       0.7,
		maxExternalRatio: 0.2,
	}
}

type FollowGraph map[string]map[string]bool

type DetectedCluster struct {
	Members         []string
	InternalDensity float64
	ExternalRatio   float64
}

func (d *ClusterDetector) Detect(ctx context.Context) ([]DetectedCluster, error) {
	log.Println("analytics: starting bot cluster detection")

	if err := d.storage.DeactivateBotClusters(ctx); err != nil {
		log.Printf("analytics: failed to deactivate old clusters: %v", err)
	}

	graph := d.buildFollowGraph(ctx)
	if len(graph) < d.minClusterSize {
		log.Printf("analytics: follow graph too small (%d nodes), skipping", len(graph))
		return nil, nil
	}

	log.Printf("analytics: built follow graph with %d nodes", len(graph))

	components := d.findSCCs(graph)
	log.Printf("analytics: found %d strongly connected components (size >= %d)", len(components), d.minClusterSize)

	clusters := d.filterBotClusters(graph, components)
	log.Printf("analytics: identified %d suspicious bot clusters", len(clusters))

	for _, cluster := range clusters {
		_, err := d.storage.SaveBotCluster(ctx, cluster.Members, cluster.InternalDensity, cluster.ExternalRatio)
		if err != nil {
			log.Printf("analytics: failed to save cluster: %v", err)
		}
	}

	return clusters, nil
}

func (d *ClusterDetector) buildFollowGraph(ctx context.Context) FollowGraph {
	graph := make(FollowGraph)

	contactLists, err := d.storage.QueryEvents(ctx, nostr.Filter{
		Kinds: []int{3},
	})
	if err != nil {
		log.Printf("analytics: failed to query contact lists: %v", err)
		return graph
	}

	latest := make(map[string]*nostr.Event)
	for _, evt := range contactLists {
		if existing, ok := latest[evt.PubKey]; !ok || evt.CreatedAt > existing.CreatedAt {
			latest[evt.PubKey] = evt
		}
	}

	for author, evt := range latest {
		if graph[author] == nil {
			graph[author] = make(map[string]bool)
		}
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				graph[author][tag[1]] = true
			}
		}
	}

	return graph
}

func (d *ClusterDetector) findSCCs(graph FollowGraph) [][]string {
	var (
		index    = 0
		stack    []string
		onStack  = make(map[string]bool)
		indices  = make(map[string]int)
		lowlinks = make(map[string]int)
		sccs     [][]string
	)

	var strongConnect func(v string)
	strongConnect = func(v string) {
		indices[v] = index
		lowlinks[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for w := range graph[v] {
			if _, visited := indices[w]; !visited {
				strongConnect(w)
				if lowlinks[w] < lowlinks[v] {
					lowlinks[v] = lowlinks[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlinks[v] {
					lowlinks[v] = indices[w]
				}
			}
		}

		if lowlinks[v] == indices[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			if len(scc) >= d.minClusterSize {
				sccs = append(sccs, scc)
			}
		}
	}

	for v := range graph {
		if _, visited := indices[v]; !visited {
			strongConnect(v)
		}
	}

	return sccs
}

func (d *ClusterDetector) filterBotClusters(graph FollowGraph, components [][]string) []DetectedCluster {
	var clusters []DetectedCluster

	for _, members := range components {
		memberSet := make(map[string]bool)
		for _, m := range members {
			memberSet[m] = true
		}

		internalEdges := 0
		externalEdges := 0

		for _, m := range members {
			for followed := range graph[m] {
				if memberSet[followed] {
					internalEdges++
				} else {
					externalEdges++
				}
			}
		}

		n := len(members)
		possibleEdges := n * (n - 1)
		if possibleEdges == 0 {
			continue
		}

		density := float64(internalEdges) / float64(possibleEdges)

		externalRatio := 0.0
		if internalEdges > 0 {
			externalRatio = float64(externalEdges) / float64(internalEdges)
		}

		if density >= d.minDensity && externalRatio <= d.maxExternalRatio {
			clusters = append(clusters, DetectedCluster{
				Members:         members,
				InternalDensity: density,
				ExternalRatio:   externalRatio,
			})
		}
	}

	return clusters
}

func (d *ClusterDetector) GetFollowGraph(ctx context.Context) FollowGraph {
	return d.buildFollowGraph(ctx)
}
