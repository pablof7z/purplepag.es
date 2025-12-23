package analytics

import (
	"context"
	"log"
	"sort"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/storage"
)

type CommunityDetector struct {
	storage        *storage.Storage
	minCommunity   int
	maxCommunities int
}

func NewCommunityDetector(store *storage.Storage) *CommunityDetector {
	return &CommunityDetector{
		storage:        store,
		minCommunity:   10,  // Minimum members to be a community
		maxCommunities: 50,  // Max communities to track
	}
}

type Community struct {
	ID              int
	Members         []string
	Size            int
	TopMembers      []CommunityMember // Top members by follower count
	InternalEdges   int
	ExternalEdges   int
	Modularity      float64
}

type CommunityMember struct {
	Pubkey        string `json:"pubkey"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	FollowerCount int    `json:"follower_count"`
}

type CommunityEdge struct {
	FromID int
	ToID   int
	Weight int // Number of cross-community follows
}

type CommunityGraph struct {
	Communities []Community
	Edges       []CommunityEdge
	TotalNodes  int
	TotalEdges  int
}

// DetectCommunities runs Louvain algorithm on the follow graph
func (d *CommunityDetector) DetectCommunities(ctx context.Context) (*CommunityGraph, error) {
	log.Println("community: starting community detection")

	// Build the follow graph
	graph := d.buildGraph(ctx)
	if len(graph.nodes) < 100 {
		log.Printf("community: graph too small (%d nodes), skipping", len(graph.nodes))
		return nil, nil
	}

	log.Printf("community: built graph with %d nodes and %d edges", len(graph.nodes), graph.edgeCount)

	// Run Louvain algorithm
	communities := d.louvain(graph)
	log.Printf("community: found %d communities", len(communities))

	// Build the community graph for visualization
	result := d.buildCommunityGraph(ctx, graph, communities)

	// Persist to database
	if err := d.storage.SaveCommunities(ctx, result); err != nil {
		log.Printf("community: failed to save communities: %v", err)
	}

	return result, nil
}

// Internal graph representation for Louvain
type louvainGraph struct {
	nodes     []string                    // node index -> pubkey
	nodeIndex map[string]int              // pubkey -> node index
	adj       []map[int]int               // adjacency list with weights
	degree    []int                       // degree of each node
	edgeCount int
}

func (d *CommunityDetector) buildGraph(ctx context.Context) *louvainGraph {
	// Get the follow graph from cluster detector
	followGraph := make(FollowGraph)

	contactLists, err := d.storage.QueryEvents(ctx, nostr.Filter{
		Kinds: []int{3},
	})
	if err != nil {
		log.Printf("community: failed to query contact lists: %v", err)
		return &louvainGraph{nodeIndex: make(map[string]int)}
	}

	// Keep only latest contact list per pubkey
	latest := make(map[string]*nostr.Event)
	for _, evt := range contactLists {
		if existing, ok := latest[evt.PubKey]; !ok || evt.CreatedAt > existing.CreatedAt {
			latest[evt.PubKey] = evt
		}
	}

	// Build follow graph
	for author, evt := range latest {
		if followGraph[author] == nil {
			followGraph[author] = make(map[string]bool)
		}
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				followGraph[author][tag[1]] = true
			}
		}
	}

	// Convert to Louvain format - treat as undirected for community detection
	nodeSet := make(map[string]bool)
	for from := range followGraph {
		nodeSet[from] = true
		for to := range followGraph[from] {
			nodeSet[to] = true
		}
	}

	nodes := make([]string, 0, len(nodeSet))
	nodeIndex := make(map[string]int)
	for node := range nodeSet {
		nodeIndex[node] = len(nodes)
		nodes = append(nodes, node)
	}

	adj := make([]map[int]int, len(nodes))
	for i := range adj {
		adj[i] = make(map[int]int)
	}

	degree := make([]int, len(nodes))
	edgeCount := 0

	// Create undirected edges (mutual follows get weight 2)
	for from, follows := range followGraph {
		fromIdx := nodeIndex[from]
		for to := range follows {
			toIdx := nodeIndex[to]
			if fromIdx != toIdx {
				adj[fromIdx][toIdx]++
				adj[toIdx][fromIdx]++
				degree[fromIdx]++
				degree[toIdx]++
				edgeCount++
			}
		}
	}

	return &louvainGraph{
		nodes:     nodes,
		nodeIndex: nodeIndex,
		adj:       adj,
		degree:    degree,
		edgeCount: edgeCount,
	}
}

// louvain implements the Louvain community detection algorithm
func (d *CommunityDetector) louvain(g *louvainGraph) map[string]int {
	n := len(g.nodes)
	if n == 0 {
		return make(map[string]int)
	}

	// Initialize: each node in its own community
	community := make([]int, n)
	for i := range community {
		community[i] = i
	}

	m2 := float64(g.edgeCount * 2) // 2m for modularity calculation
	if m2 == 0 {
		m2 = 1
	}

	// Community stats
	communityDegree := make([]float64, n) // sum of degrees in community
	for i := range communityDegree {
		communityDegree[i] = float64(g.degree[i])
	}

	improved := true
	iterations := 0
	maxIterations := 20

	for improved && iterations < maxIterations {
		improved = false
		iterations++

		// Try moving each node to a neighboring community
		for i := 0; i < n; i++ {
			currentCom := community[i]
			bestCom := currentCom
			bestDelta := 0.0

			ki := float64(g.degree[i])

			// Calculate communities of neighbors
			neighborComs := make(map[int]float64) // community -> edge weight to that community
			for j, w := range g.adj[i] {
				neighborComs[community[j]] += float64(w)
			}

			// Remove node from current community temporarily
			communityDegree[currentCom] -= ki

			for com, kiIn := range neighborComs {
				sigmaTot := communityDegree[com]
				if com == currentCom {
					sigmaTot = communityDegree[currentCom]
				}

				// Modularity gain for moving to this community
				delta := kiIn/m2 - (sigmaTot*ki)/(m2*m2)*2
				if delta > bestDelta {
					bestDelta = delta
					bestCom = com
				}
			}

			// Also consider staying in current community
			if currentCom != bestCom {
				kiInCurrent := neighborComs[currentCom]
				sigmaTotCurrent := communityDegree[currentCom]
				deltaCurrent := kiInCurrent/m2 - (sigmaTotCurrent*ki)/(m2*m2)*2
				if deltaCurrent >= bestDelta {
					bestCom = currentCom
					bestDelta = deltaCurrent
				}
			}

			// Move to best community
			communityDegree[currentCom] += ki // restore temporarily removed
			if bestCom != currentCom {
				community[i] = bestCom
				communityDegree[currentCom] -= ki
				communityDegree[bestCom] += ki
				improved = true
			}
		}
	}

	log.Printf("community: Louvain converged after %d iterations", iterations)

	// Renumber communities to be contiguous
	comMap := make(map[int]int)
	nextCom := 0
	result := make(map[string]int)

	for i, com := range community {
		if _, ok := comMap[com]; !ok {
			comMap[com] = nextCom
			nextCom++
		}
		result[g.nodes[i]] = comMap[com]
	}

	return result
}

func (d *CommunityDetector) buildCommunityGraph(ctx context.Context, g *louvainGraph, communities map[string]int) *CommunityGraph {
	// Group members by community
	communityMembers := make(map[int][]string)
	for pubkey, comID := range communities {
		communityMembers[comID] = append(communityMembers[comID], pubkey)
	}

	// Get real follower counts from database (minimum 1 follower)
	followerCount, err := d.storage.GetFollowerCounts(ctx, 1)
	if err != nil {
		log.Printf("community: failed to get follower counts: %v", err)
		followerCount = make(map[string]int)
	}

	// Build community list first to identify top candidates
	var comList []Community
	for comID, members := range communityMembers {
		if len(members) < d.minCommunity {
			continue
		}

		// Sort members by follower count
		sort.Slice(members, func(i, j int) bool {
			return followerCount[members[i]] > followerCount[members[j]]
		})

		comList = append(comList, Community{
			ID:      comID,
			Members: members,
			Size:    len(members),
		})
	}

	// Collect top member pubkeys to fetch profiles for
	topPubkeys := make([]string, 0)
	for _, com := range comList {
		topN := 5
		if len(com.Members) < topN {
			topN = len(com.Members)
		}
		for i := 0; i < topN; i++ {
			topPubkeys = append(topPubkeys, com.Members[i])
		}
	}

	// Get profile info (names + pictures) for top members only
	profiles, _ := d.storage.GetProfileInfo(ctx, topPubkeys)

	// Now build top members with profile info
	for i := range comList {
		topN := 5
		if len(comList[i].Members) < topN {
			topN = len(comList[i].Members)
		}
		topMembers := make([]CommunityMember, topN)
		for j := 0; j < topN; j++ {
			pk := comList[i].Members[j]
			profile := profiles[pk]
			topMembers[j] = CommunityMember{
				Pubkey:        pk,
				Name:          profile.Name,
				Picture:       profile.Picture,
				FollowerCount: followerCount[pk],
			}
		}
		comList[i].TopMembers = topMembers
	}

	// Sort by size descending
	sort.Slice(comList, func(i, j int) bool {
		return comList[i].Size > comList[j].Size
	})

	// Keep top N communities
	if len(comList) > d.maxCommunities {
		comList = comList[:d.maxCommunities]
	}

	// Reassign IDs
	newComID := make(map[int]int)
	for i := range comList {
		newComID[comList[i].ID] = i
		comList[i].ID = i
	}

	// Calculate edges between communities
	edgeWeights := make(map[[2]int]int)
	for i, neighbors := range g.adj {
		fromPk := g.nodes[i]
		fromCom, ok := communities[fromPk]
		if !ok {
			continue
		}
		newFromCom, ok := newComID[fromCom]
		if !ok {
			continue
		}

		for j := range neighbors {
			toPk := g.nodes[j]
			toCom, ok := communities[toPk]
			if !ok {
				continue
			}
			newToCom, ok := newComID[toCom]
			if !ok {
				continue
			}

			if newFromCom != newToCom {
				key := [2]int{min(newFromCom, newToCom), max(newFromCom, newToCom)}
				edgeWeights[key]++
			}
		}
	}

	// Build edge list
	var edges []CommunityEdge
	for key, weight := range edgeWeights {
		edges = append(edges, CommunityEdge{
			FromID: key[0],
			ToID:   key[1],
			Weight: weight,
		})
	}

	// Sort edges by weight
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].Weight > edges[j].Weight
	})

	// Keep top edges (avoid cluttered visualization)
	maxEdges := len(comList) * 3
	if len(edges) > maxEdges {
		edges = edges[:maxEdges]
	}

	// Calculate modularity for each community
	for i := range comList {
		internalEdges := 0
		externalEdges := 0
		memberSet := make(map[string]bool)
		for _, m := range comList[i].Members {
			memberSet[m] = true
		}

		for _, m := range comList[i].Members {
			idx := g.nodeIndex[m]
			for neighborIdx := range g.adj[idx] {
				neighborPk := g.nodes[neighborIdx]
				if memberSet[neighborPk] {
					internalEdges++
				} else {
					externalEdges++
				}
			}
		}

		comList[i].InternalEdges = internalEdges / 2 // counted twice
		comList[i].ExternalEdges = externalEdges

		// Simple modularity approximation
		if internalEdges+externalEdges > 0 {
			comList[i].Modularity = float64(internalEdges) / float64(internalEdges+externalEdges)
		}
	}

	return &CommunityGraph{
		Communities: comList,
		Edges:       edges,
		TotalNodes:  len(g.nodes),
		TotalEdges:  g.edgeCount,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// CalculateModularity calculates the overall modularity of the partition
func (d *CommunityDetector) CalculateModularity(g *louvainGraph, communities map[string]int) float64 {
	m2 := float64(g.edgeCount * 2)
	if m2 == 0 {
		return 0
	}

	// Sum of degrees in each community
	communityDegree := make(map[int]float64)
	communityInternal := make(map[int]float64)

	for i, pk := range g.nodes {
		com := communities[pk]
		communityDegree[com] += float64(g.degree[i])

		for j := range g.adj[i] {
			if communities[g.nodes[j]] == com {
				communityInternal[com]++
			}
		}
	}

	Q := 0.0
	for com := range communityDegree {
		eii := communityInternal[com] / m2
		ai := communityDegree[com] / m2
		Q += eii - ai*ai
	}

	return Q
}
