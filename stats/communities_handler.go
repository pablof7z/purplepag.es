package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/pablof7z/purplepag.es/storage"
)

type CommunitiesHandler struct {
	storage *storage.Storage
}

func NewCommunitiesHandler(store *storage.Storage) *CommunitiesHandler {
	return &CommunitiesHandler{storage: store}
}

type CommunityPageData struct {
	Graph         *storage.StoredCommunityGraph
	GraphJSON     template.JS
	DetectedAgo   string
	HasData       bool
}

func (h *CommunitiesHandler) HandleCommunities() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		data := CommunityPageData{}

		graph, err := h.storage.GetCommunityGraph(ctx)
		if err == nil && graph != nil && len(graph.Communities) > 0 {
			data.Graph = graph
			data.HasData = true
			data.DetectedAgo = communityFormatTimeAgo(time.Since(graph.DetectedAt))

			// Convert to JSON for D3.js
			graphData := map[string]interface{}{
				"nodes": h.buildNodes(graph),
				"links": h.buildLinks(graph),
			}
			jsonBytes, _ := json.Marshal(graphData)
			data.GraphJSON = template.JS(jsonBytes)
		}

		tmpl, err := template.New("communities").Parse(communitiesTemplate)
		if err != nil {
			http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func (h *CommunitiesHandler) HandleCommunityMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		communityID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "Invalid community ID", http.StatusBadRequest)
			return
		}

		members, err := h.storage.GetCommunityMembers(ctx, communityID, 100)
		if err != nil {
			http.Error(w, "Failed to get members", http.StatusInternalServerError)
			return
		}

		// Get names for the members
		names, _ := h.storage.GetProfileNames(ctx, members)

		type memberInfo struct {
			Pubkey string `json:"pubkey"`
			Name   string `json:"name"`
		}

		result := make([]memberInfo, len(members))
		for i, pk := range members {
			result[i] = memberInfo{
				Pubkey: pk,
				Name:   names[pk],
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func (h *CommunitiesHandler) buildNodes(graph *storage.StoredCommunityGraph) []map[string]interface{} {
	nodes := make([]map[string]interface{}, len(graph.Communities))
	for i, com := range graph.Communities {
		// Build label from top members
		label := ""
		for j, m := range com.TopMembers {
			if j > 0 {
				label += ", "
			}
			if m.Name != "" {
				label += m.Name
			} else if len(m.Pubkey) > 8 {
				label += m.Pubkey[:8] + "..."
			}
			if j >= 2 {
				break
			}
		}

		nodes[i] = map[string]interface{}{
			"id":            com.ID,
			"size":          com.Size,
			"label":         label,
			"modularity":    com.Modularity,
			"internalEdges": com.InternalEdges,
			"externalEdges": com.ExternalEdges,
			"topMembers":    com.TopMembers,
		}
	}
	return nodes
}

func (h *CommunitiesHandler) buildLinks(graph *storage.StoredCommunityGraph) []map[string]interface{} {
	links := make([]map[string]interface{}, len(graph.Edges))
	for i, edge := range graph.Edges {
		links[i] = map[string]interface{}{
			"source": edge.FromID,
			"target": edge.ToID,
			"weight": edge.Weight,
		}
	}
	return links
}

func communityFormatTimeAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

var communitiesTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Social Graph Communities</title>
    <script src="https://d3js.org/d3.v7.min.js"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, sans-serif;
            background: #0d1117;
            min-height: 100vh;
            color: #c9d1d9;
        }
        .header {
            background: #161b22;
            border-bottom: 1px solid #30363d;
            padding: 1rem 2rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .header h1 {
            font-size: 1.5rem;
            font-weight: 600;
            color: #f0f6fc;
        }
        .header .stats {
            display: flex;
            gap: 2rem;
            font-size: 0.875rem;
            color: #8b949e;
        }
        .header .stats span {
            color: #58a6ff;
            font-weight: 600;
        }
        .back-link {
            color: #58a6ff;
            text-decoration: none;
            font-size: 0.875rem;
        }
        .back-link:hover { text-decoration: underline; }

        #graph-container {
            width: 100%;
            height: calc(100vh - 120px);
            position: relative;
        }

        #graph {
            width: 100%;
            height: 100%;
        }

        .node circle {
            stroke: #30363d;
            stroke-width: 2px;
            cursor: pointer;
            transition: stroke 0.2s;
        }
        .node circle:hover {
            stroke: #58a6ff;
            stroke-width: 3px;
        }
        .node text {
            font-size: 10px;
            fill: #c9d1d9;
            pointer-events: none;
            text-anchor: middle;
        }
        .link {
            stroke: #30363d;
            stroke-opacity: 0.6;
        }

        .tooltip {
            position: absolute;
            background: #21262d;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 12px;
            font-size: 0.875rem;
            pointer-events: none;
            opacity: 0;
            transition: opacity 0.2s;
            max-width: 300px;
            z-index: 100;
        }
        .tooltip.visible { opacity: 1; }
        .tooltip h3 {
            font-size: 1rem;
            color: #f0f6fc;
            margin-bottom: 8px;
        }
        .tooltip .stat {
            display: flex;
            justify-content: space-between;
            margin: 4px 0;
        }
        .tooltip .stat-label { color: #8b949e; }
        .tooltip .stat-value { color: #58a6ff; font-weight: 600; }
        .tooltip .members {
            margin-top: 8px;
            padding-top: 8px;
            border-top: 1px solid #30363d;
        }
        .tooltip .member {
            color: #c9d1d9;
            margin: 2px 0;
        }

        .legend {
            position: absolute;
            bottom: 20px;
            left: 20px;
            background: #21262d;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 12px;
            font-size: 0.75rem;
        }
        .legend-title {
            font-weight: 600;
            margin-bottom: 8px;
            color: #f0f6fc;
        }
        .legend-item {
            display: flex;
            align-items: center;
            gap: 8px;
            margin: 4px 0;
        }
        .legend-circle {
            border-radius: 50%;
        }

        .controls {
            position: absolute;
            top: 20px;
            right: 20px;
            background: #21262d;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 12px;
        }
        .controls button {
            background: #30363d;
            border: 1px solid #484f58;
            color: #c9d1d9;
            padding: 6px 12px;
            border-radius: 6px;
            cursor: pointer;
            margin: 2px;
            font-size: 0.75rem;
        }
        .controls button:hover {
            background: #484f58;
        }

        .empty-state {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            height: calc(100vh - 120px);
            color: #8b949e;
        }
        .empty-state h2 {
            font-size: 1.5rem;
            margin-bottom: 1rem;
            color: #c9d1d9;
        }
    </style>
</head>
<body>
    <div class="header">
        <div>
            <a href="/stats" class="back-link">← Back to Stats</a>
            <h1>Social Graph Communities</h1>
        </div>
        {{if .HasData}}
        <div class="stats">
            <div>Communities: <span>{{.Graph.NumCommunities}}</span></div>
            <div>Total Nodes: <span>{{.Graph.TotalNodes}}</span></div>
            <div>Total Edges: <span>{{.Graph.TotalEdges}}</span></div>
            <div>Updated: <span>{{.DetectedAgo}}</span></div>
        </div>
        {{end}}
    </div>

    {{if .HasData}}
    <div id="graph-container">
        <svg id="graph"></svg>
        <div class="tooltip" id="tooltip"></div>

        <div class="legend">
            <div class="legend-title">Community Size</div>
            <div class="legend-item">
                <div class="legend-circle" style="width:10px;height:10px;background:#238636"></div>
                <span>Small (&lt;100)</span>
            </div>
            <div class="legend-item">
                <div class="legend-circle" style="width:15px;height:15px;background:#1f6feb"></div>
                <span>Medium (100-500)</span>
            </div>
            <div class="legend-item">
                <div class="legend-circle" style="width:20px;height:20px;background:#a371f7"></div>
                <span>Large (&gt;500)</span>
            </div>
        </div>

        <div class="controls">
            <button onclick="resetZoom()">Reset View</button>
            <button onclick="toggleLabels()">Toggle Labels</button>
        </div>
    </div>

    <script>
        const graphData = {{.GraphJSON}};

        const container = document.getElementById('graph-container');
        const width = container.clientWidth;
        const height = container.clientHeight;

        const svg = d3.select('#graph')
            .attr('width', width)
            .attr('height', height);

        const g = svg.append('g');

        // Zoom behavior
        const zoom = d3.zoom()
            .scaleExtent([0.1, 4])
            .on('zoom', (event) => g.attr('transform', event.transform));

        svg.call(zoom);

        function resetZoom() {
            svg.transition().duration(750).call(zoom.transform, d3.zoomIdentity);
        }

        let showLabels = true;
        function toggleLabels() {
            showLabels = !showLabels;
            g.selectAll('.node text').style('opacity', showLabels ? 1 : 0);
        }

        // Size scale
        const sizeScale = d3.scaleSqrt()
            .domain([0, d3.max(graphData.nodes, d => d.size)])
            .range([8, 60]);

        // Color scale based on size
        function getColor(size) {
            if (size < 100) return '#238636';
            if (size < 500) return '#1f6feb';
            return '#a371f7';
        }

        // Link width scale
        const linkScale = d3.scaleSqrt()
            .domain([0, d3.max(graphData.links, d => d.weight) || 1])
            .range([1, 8]);

        // Create force simulation
        const simulation = d3.forceSimulation(graphData.nodes)
            .force('link', d3.forceLink(graphData.links)
                .id(d => d.id)
                .distance(d => 100 + sizeScale(d.source.size || 0) + sizeScale(d.target.size || 0))
                .strength(0.5))
            .force('charge', d3.forceManyBody()
                .strength(d => -sizeScale(d.size) * 20))
            .force('center', d3.forceCenter(width / 2, height / 2))
            .force('collision', d3.forceCollide()
                .radius(d => sizeScale(d.size) + 10));

        // Draw links
        const link = g.append('g')
            .selectAll('line')
            .data(graphData.links)
            .join('line')
            .attr('class', 'link')
            .attr('stroke-width', d => linkScale(d.weight));

        // Draw nodes
        const node = g.append('g')
            .selectAll('.node')
            .data(graphData.nodes)
            .join('g')
            .attr('class', 'node')
            .call(d3.drag()
                .on('start', dragstarted)
                .on('drag', dragged)
                .on('end', dragended));

        node.append('circle')
            .attr('r', d => sizeScale(d.size))
            .attr('fill', d => getColor(d.size))
            .on('mouseover', showTooltip)
            .on('mouseout', hideTooltip)
            .on('click', (event, d) => {
                window.open('/profile?pubkey=' + d.topMembers[0]?.pubkey, '_blank');
            });

        node.append('text')
            .attr('dy', d => sizeScale(d.size) + 12)
            .text(d => d.label);

        // Tooltip
        const tooltip = d3.select('#tooltip');

        function showTooltip(event, d) {
            let membersHtml = '<div class="members"><strong>Top Members:</strong>';
            d.topMembers.forEach(m => {
                const name = m.name || m.pubkey.substring(0, 12) + '...';
                membersHtml += '<div class="member">' + name + ' (' + m.follower_count + ' followers)</div>';
            });
            membersHtml += '</div>';

            tooltip.html(
                '<h3>Community #' + d.id + '</h3>' +
                '<div class="stat"><span class="stat-label">Members</span><span class="stat-value">' + d.size + '</span></div>' +
                '<div class="stat"><span class="stat-label">Internal Edges</span><span class="stat-value">' + d.internalEdges + '</span></div>' +
                '<div class="stat"><span class="stat-label">External Edges</span><span class="stat-value">' + d.externalEdges + '</span></div>' +
                '<div class="stat"><span class="stat-label">Cohesion</span><span class="stat-value">' + (d.modularity * 100).toFixed(1) + '%</span></div>' +
                membersHtml
            )
            .style('left', (event.pageX + 10) + 'px')
            .style('top', (event.pageY - 10) + 'px')
            .classed('visible', true);
        }

        function hideTooltip() {
            tooltip.classed('visible', false);
        }

        // Simulation tick
        simulation.on('tick', () => {
            link
                .attr('x1', d => d.source.x)
                .attr('y1', d => d.source.y)
                .attr('x2', d => d.target.x)
                .attr('y2', d => d.target.y);

            node.attr('transform', d => 'translate(' + d.x + ',' + d.y + ')');
        });

        // Drag functions
        function dragstarted(event) {
            if (!event.active) simulation.alphaTarget(0.3).restart();
            event.subject.fx = event.subject.x;
            event.subject.fy = event.subject.y;
        }

        function dragged(event) {
            event.subject.fx = event.x;
            event.subject.fy = event.y;
        }

        function dragended(event) {
            if (!event.active) simulation.alphaTarget(0);
            event.subject.fx = null;
            event.subject.fy = null;
        }
    </script>
    {{else}}
    <div class="empty-state">
        <h2>No Community Data Yet</h2>
        <p>Community detection runs periodically. Check back later.</p>
    </div>
    {{end}}
</body>
</html>`
