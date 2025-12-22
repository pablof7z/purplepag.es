package stats

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/pablof7z/purplepag.es/analytics"
	"github.com/pablof7z/purplepag.es/storage"
)

type AnalyticsHandler struct {
	tracker       *analytics.Tracker
	trustAnalyzer *analytics.TrustAnalyzer
	storage       *storage.Storage
}

func NewAnalyticsHandler(tracker *analytics.Tracker, trustAnalyzer *analytics.TrustAnalyzer, store *storage.Storage) *AnalyticsHandler {
	return &AnalyticsHandler{
		tracker:       tracker,
		trustAnalyzer: trustAnalyzer,
		storage:       store,
	}
}

type PubkeyDisplay struct {
	Pubkey        string
	ShortPubkey   string
	Name          string
	TotalRequests int64
	LastRequest   string
	IsTrusted     bool
	IsInCluster   bool
}

type CooccurrenceDisplay struct {
	PubkeyA      string
	PubkeyB      string
	ShortPubkeyA string
	ShortPubkeyB string
	Count        int64
}

type ClusterDisplay struct {
	ID              int64
	Size            int
	InternalDensity string
	ExternalRatio   string
	DetectedAgo     string
	MemberPreviews  []string
}

type SpamDisplay struct {
	Pubkey      string
	ShortPubkey string
	Reason      string
	EventCount  int64
	DetectedAgo string
}

type AnalyticsPageData struct {
	SearchPubkey   string
	SearchResult   *PubkeyDisplay
	TopRequested   []PubkeyDisplay
	TopCooccurring []CooccurrenceDisplay
	BotClusters    []ClusterDisplay
	SpamCandidates []SpamDisplay
	TrustedCount   int
	Message        string
	Error          string
}

func (h *AnalyticsHandler) HandleAnalytics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		data := AnalyticsPageData{
			TrustedCount: h.trustAnalyzer.GetTrustedCount(),
			Message:      r.URL.Query().Get("message"),
		}

		if pubkey := r.URL.Query().Get("pubkey"); pubkey != "" {
			data.SearchPubkey = pubkey
			stats, err := h.tracker.GetPubkeyStats(ctx, pubkey)
			if err == nil && stats != nil {
				inCluster, _ := h.storage.IsPubkeyInBotCluster(ctx, pubkey)
				data.SearchResult = &PubkeyDisplay{
					Pubkey:        stats.Pubkey,
					ShortPubkey:   shortPubkey(stats.Pubkey),
					TotalRequests: stats.TotalRequests,
					LastRequest:   formatTimeAgo(time.Since(stats.LastRequest)),
					IsTrusted:     h.trustAnalyzer.IsTrusted(pubkey),
					IsInCluster:   inCluster,
				}
			}
		}

		topRequested, _ := h.tracker.GetTopRequested(ctx, 50)

		// Fetch profile names for top requested pubkeys
		topPubkeys := make([]string, len(topRequested))
		for i, s := range topRequested {
			topPubkeys[i] = s.Pubkey
		}
		profileNames, _ := h.storage.GetProfileNames(ctx, topPubkeys)

		for _, s := range topRequested {
			inCluster, _ := h.storage.IsPubkeyInBotCluster(ctx, s.Pubkey)
			data.TopRequested = append(data.TopRequested, PubkeyDisplay{
				Pubkey:        s.Pubkey,
				ShortPubkey:   shortPubkey(s.Pubkey),
				Name:          profileNames[s.Pubkey],
				TotalRequests: s.TotalRequests,
				LastRequest:   formatTimeAgo(time.Since(s.LastRequest)),
				IsTrusted:     h.trustAnalyzer.IsTrusted(s.Pubkey),
				IsInCluster:   inCluster,
			})
		}

		topCooccur, _ := h.tracker.GetTopCooccurring(ctx, 50)
		for _, c := range topCooccur {
			data.TopCooccurring = append(data.TopCooccurring, CooccurrenceDisplay{
				PubkeyA:      c.PubkeyA,
				PubkeyB:      c.PubkeyB,
				ShortPubkeyA: shortPubkey(c.PubkeyA),
				ShortPubkeyB: shortPubkey(c.PubkeyB),
				Count:        c.Count,
			})
		}

		clusters, _ := h.storage.GetBotClusters(ctx, 20)
		for _, c := range clusters {
			display := ClusterDisplay{
				ID:              c.ID,
				Size:            c.Size,
				InternalDensity: fmt.Sprintf("%.1f%%", c.InternalDensity*100),
				ExternalRatio:   fmt.Sprintf("%.1f%%", c.ExternalRatio*100),
				DetectedAgo:     formatTimeAgo(time.Since(c.DetectedAt)),
			}
			for i, m := range c.Members {
				if i >= 5 {
					break
				}
				display.MemberPreviews = append(display.MemberPreviews, shortPubkey(m))
			}
			data.BotClusters = append(data.BotClusters, display)
		}

		spamCandidates, _ := h.trustAnalyzer.GetSpamCandidates(ctx, 100)
		for _, c := range spamCandidates {
			data.SpamCandidates = append(data.SpamCandidates, SpamDisplay{
				Pubkey:      c.Pubkey,
				ShortPubkey: shortPubkey(c.Pubkey),
				Reason:      c.Reason,
				EventCount:  c.EventCount,
				DetectedAgo: formatTimeAgo(time.Since(c.DetectedAt)),
			})
		}

		tmpl, err := template.New("analytics").Parse(analyticsTemplate)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func (h *AnalyticsHandler) HandlePurge() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := context.Background()

		spamCandidates, err := h.trustAnalyzer.GetSpamCandidates(ctx, 10000)
		if err != nil {
			http.Error(w, "Failed to get spam candidates", http.StatusInternalServerError)
			return
		}

		if len(spamCandidates) == 0 {
			http.Redirect(w, r, "/stats/analytics?message=No+spam+candidates+to+purge", http.StatusSeeOther)
			return
		}

		pubkeys := make([]string, len(spamCandidates))
		for i, c := range spamCandidates {
			pubkeys[i] = c.Pubkey
		}

		deleted, err := h.storage.DeleteEventsForPubkeys(ctx, pubkeys)
		if err != nil {
			http.Error(w, "Failed to delete events", http.StatusInternalServerError)
			return
		}

		if err := h.storage.MarkSpamPurged(ctx, pubkeys); err != nil {
			http.Error(w, "Failed to mark as purged", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/stats/analytics?message=Purged+%d+events+from+%d+spam+pubkeys", deleted, len(pubkeys)), http.StatusSeeOther)
	}
}

func shortPubkey(pubkey string) string {
	if len(pubkey) <= 16 {
		return pubkey
	}
	return pubkey[:8] + "..." + pubkey[len(pubkey)-8:]
}

var analyticsTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - REQ Analytics</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            background: #0d1117;
            min-height: 100vh;
            padding: 2rem;
            color: #c9d1d9;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        header { margin-bottom: 2rem; border-bottom: 1px solid #21262d; padding-bottom: 1rem; }
        h1 { font-size: 1.5rem; font-weight: 600; color: #f0f6fc; margin-bottom: 0.25rem; }
        .subtitle { font-size: 0.875rem; color: #8b949e; }
        .back-link { display: inline-block; margin-bottom: 1rem; color: #58a6ff; text-decoration: none; font-size: 0.875rem; }
        .back-link:hover { text-decoration: underline; }

        .search-box {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            margin-bottom: 1.5rem;
        }
        .search-box input {
            width: 100%;
            padding: 0.5rem;
            font-size: 0.875rem;
            background: #0d1117;
            border: 1px solid #30363d;
            border-radius: 6px;
            color: #c9d1d9;
            font-family: inherit;
        }
        .search-box input:focus { outline: none; border-color: #58a6ff; }
        .search-box button {
            margin-top: 0.75rem;
            padding: 0.5rem 1rem;
            background: #21262d;
            border: 1px solid #30363d;
            border-radius: 6px;
            color: #c9d1d9;
            font-family: inherit;
            font-size: 0.875rem;
            cursor: pointer;
        }
        .search-box button:hover { background: #30363d; }

        .result-card {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            margin-bottom: 1.5rem;
        }
        .result-card h3 { color: #f0f6fc; margin-bottom: 0.75rem; font-size: 0.875rem; }
        .result-card .pubkey { font-size: 0.75rem; color: #8b949e; word-break: break-all; }
        .result-card .stats { display: flex; gap: 2rem; margin-top: 0.75rem; }
        .result-card .stat-label { font-size: 0.625rem; color: #8b949e; text-transform: uppercase; }
        .result-card .stat-value { font-size: 1.25rem; font-weight: 600; color: #f0f6fc; }
        .badge { display: inline-block; padding: 0.125rem 0.5rem; border-radius: 4px; font-size: 0.625rem; font-weight: 600; margin-left: 0.5rem; }
        .badge.trusted { background: #238636; color: #fff; }
        .badge.cluster { background: #da3633; color: #fff; }

        .section {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            margin-bottom: 1rem;
        }
        .section h2 { font-size: 0.875rem; font-weight: 600; margin-bottom: 1rem; color: #f0f6fc; }

        .data-table { width: 100%; border-collapse: collapse; }
        .data-table th, .data-table td { padding: 0.5rem; text-align: left; border-bottom: 1px solid #21262d; }
        .data-table th { color: #8b949e; font-weight: 600; font-size: 0.625rem; text-transform: uppercase; }
        .data-table td { font-size: 0.75rem; }
        .data-table .mono { color: #8b949e; }
        .data-table .num { font-variant-numeric: tabular-nums; color: #58a6ff; font-weight: 600; }

        .cluster-card {
            background: #1c1210;
            border: 1px solid #f8514966;
            border-radius: 6px;
            padding: 0.75rem;
            margin-bottom: 0.75rem;
        }
        .cluster-card .header { display: flex; justify-content: space-between; margin-bottom: 0.5rem; font-size: 0.75rem; }
        .cluster-card .members { font-size: 0.625rem; color: #8b949e; }

        .spam-section { background: #1c1210; border-color: #f8514966; }
        .purge-btn {
            padding: 0.5rem 1rem;
            background: #da3633;
            border: none;
            border-radius: 6px;
            color: white;
            font-weight: 600;
            font-family: inherit;
            font-size: 0.75rem;
            cursor: pointer;
            margin-bottom: 0.75rem;
        }
        .purge-btn:hover { background: #f85149; }

        .message {
            background: #122117;
            border: 1px solid #238636;
            color: #3fb950;
            padding: 0.75rem;
            border-radius: 6px;
            margin-bottom: 1.5rem;
            font-size: 0.875rem;
        }

        .stats-row { display: flex; gap: 1rem; margin-bottom: 1.5rem; }
        .stat-box {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            flex: 1;
        }
        .stat-box .label { font-size: 0.625rem; color: #8b949e; text-transform: uppercase; margin-bottom: 0.25rem; }
        .stat-box .value { font-size: 1.5rem; font-weight: 600; color: #f0f6fc; }

        @media (max-width: 768px) {
            body { padding: 1rem; }
            .stats-row { flex-direction: column; gap: 0.75rem; }
            .data-table { font-size: 0.625rem; }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats" class="back-link">← Back to Stats</a>
        <header>
            <h1>purplepag.es</h1>
            <div class="subtitle">REQ Analytics & Spam Detection</div>
        </header>

        {{if .Message}}
        <div class="message">{{.Message}}</div>
        {{end}}

        <div class="stats-row">
            <div class="stat-box">
                <div class="label">Trusted Pubkeys</div>
                <div class="value">{{.TrustedCount}}</div>
            </div>
            <div class="stat-box">
                <div class="label">Bot Clusters</div>
                <div class="value">{{len .BotClusters}}</div>
            </div>
            <div class="stat-box">
                <div class="label">Spam Candidates</div>
                <div class="value">{{len .SpamCandidates}}</div>
            </div>
        </div>

        <div class="search-box">
            <form method="GET">
                <input type="text" name="pubkey" placeholder="Search pubkey..." value="{{.SearchPubkey}}">
                <button type="submit">Search</button>
            </form>
        </div>

        {{if .SearchResult}}
        <div class="result-card">
            <h3>Search Result
                {{if .SearchResult.IsTrusted}}<span class="badge trusted">Trusted</span>{{end}}
                {{if .SearchResult.IsInCluster}}<span class="badge cluster">Bot Cluster</span>{{end}}
            </h3>
            <div class="pubkey">{{.SearchResult.Pubkey}}</div>
            <div class="stats">
                <div class="stat">
                    <div class="stat-label">Total Requests</div>
                    <div class="stat-value">{{.SearchResult.TotalRequests}}</div>
                </div>
                <div class="stat">
                    <div class="stat-label">Last Requested</div>
                    <div class="stat-value">{{.SearchResult.LastRequest}}</div>
                </div>
            </div>
        </div>
        {{end}}

        {{if .SpamCandidates}}
        <div class="section spam-section">
            <h2>Spam Candidates ({{len .SpamCandidates}})</h2>
            <form method="POST" action="/stats/analytics/purge" onsubmit="return confirm('Are you sure you want to purge all spam events? This cannot be undone.');">
                <button type="submit" class="purge-btn">Purge All Spam Events</button>
            </form>
            <table class="data-table">
                <thead>
                    <tr>
                        <th>Pubkey</th>
                        <th>Reason</th>
                        <th>Events</th>
                        <th>Detected</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .SpamCandidates}}
                    <tr>
                        <td class="mono">{{.ShortPubkey}}</td>
                        <td>{{.Reason}}</td>
                        <td class="num">{{.EventCount}}</td>
                        <td>{{.DetectedAgo}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}

        {{if .BotClusters}}
        <div class="section">
            <h2>Detected Bot Clusters</h2>
            {{range .BotClusters}}
            <div class="cluster-card">
                <div class="header">
                    <strong>Cluster #{{.ID}}</strong>
                    <span>{{.Size}} members · {{.InternalDensity}} density · {{.ExternalRatio}} external · {{.DetectedAgo}}</span>
                </div>
                <div class="members">{{range .MemberPreviews}}{{.}} {{end}}{{if gt .Size 5}}...{{end}}</div>
            </div>
            {{end}}
        </div>
        {{end}}

        {{if .TopRequested}}
        <div class="section">
            <h2>Top Requested Pubkeys</h2>
            <table class="data-table">
                <thead>
                    <tr>
                        <th>Name / Pubkey</th>
                        <th>Requests</th>
                        <th>Last Requested</th>
                        <th>Status</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .TopRequested}}
                    <tr>
                        <td>
                            {{if .Name}}<strong style="color:#f0f6fc">{{.Name}}</strong><br>{{end}}<span class="mono" style="font-size:0.65rem">{{.ShortPubkey}}</span>
                        </td>
                        <td class="num">{{.TotalRequests}}</td>
                        <td>{{.LastRequest}}</td>
                        <td>
                            {{if .IsTrusted}}<span class="badge trusted">Trusted</span>{{end}}
                            {{if .IsInCluster}}<span class="badge cluster">Bot Cluster</span>{{end}}
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}

        {{if .TopCooccurring}}
        <div class="section">
            <h2>Pubkey Co-occurrence Graph</h2>
            <p style="color: #8b949e; margin-bottom: 0.75rem; font-size: 0.75rem;">
                Nodes represent pubkeys, edges show co-occurrence frequency. Drag to reposition, scroll to zoom.
            </p>
            <div id="graph-container" style="width: 100%; height: 600px; background: #0d1117; border-radius: 6px; overflow: hidden;"></div>
        </div>
        <script src="https://d3js.org/d3.v7.min.js"></script>
        <script>
        (function() {
            const cooccurrences = [
                {{range .TopCooccurring}}
                {a: "{{.ShortPubkeyA}}", aFull: "{{.PubkeyA}}", b: "{{.ShortPubkeyB}}", bFull: "{{.PubkeyB}}", count: {{.Count}}},
                {{end}}
            ];

            const nodeMap = new Map();
            const links = [];

            cooccurrences.forEach(c => {
                if (!nodeMap.has(c.a)) {
                    nodeMap.set(c.a, {id: c.a, full: c.aFull, weight: 0});
                }
                if (!nodeMap.has(c.b)) {
                    nodeMap.set(c.b, {id: c.b, full: c.bFull, weight: 0});
                }
                nodeMap.get(c.a).weight += c.count;
                nodeMap.get(c.b).weight += c.count;
                links.push({source: c.a, target: c.b, value: c.count});
            });

            const nodes = Array.from(nodeMap.values());
            const maxWeight = Math.max(...nodes.map(n => n.weight));
            const maxLink = Math.max(...links.map(l => l.value));

            const container = document.getElementById('graph-container');
            const width = container.clientWidth;
            const height = 600;

            const svg = d3.select('#graph-container')
                .append('svg')
                .attr('width', width)
                .attr('height', height);

            const g = svg.append('g');

            const zoom = d3.zoom()
                .scaleExtent([0.2, 4])
                .on('zoom', (event) => g.attr('transform', event.transform));

            svg.call(zoom);

            const simulation = d3.forceSimulation(nodes)
                .force('link', d3.forceLink(links).id(d => d.id).distance(d => 100 - (d.value / maxLink) * 50).strength(d => 0.3 + (d.value / maxLink) * 0.7))
                .force('charge', d3.forceManyBody().strength(-200))
                .force('center', d3.forceCenter(width / 2, height / 2))
                .force('collision', d3.forceCollide().radius(d => 10 + (d.weight / maxWeight) * 20));

            const link = g.append('g')
                .selectAll('line')
                .data(links)
                .join('line')
                .attr('stroke', '#58a6ff')
                .attr('stroke-opacity', d => 0.2 + (d.value / maxLink) * 0.6)
                .attr('stroke-width', d => 1 + (d.value / maxLink) * 4);

            const node = g.append('g')
                .selectAll('g')
                .data(nodes)
                .join('g')
                .call(d3.drag()
                    .on('start', (event, d) => {
                        if (!event.active) simulation.alphaTarget(0.3).restart();
                        d.fx = d.x; d.fy = d.y;
                    })
                    .on('drag', (event, d) => { d.fx = event.x; d.fy = event.y; })
                    .on('end', (event, d) => {
                        if (!event.active) simulation.alphaTarget(0);
                        d.fx = null; d.fy = null;
                    }));

            node.append('circle')
                .attr('r', d => 6 + (d.weight / maxWeight) * 14)
                .attr('fill', d => {
                    const t = d.weight / maxWeight;
                    return d3.interpolateRgb('#388bfd', '#58a6ff')(t);
                })
                .attr('stroke', '#0d1117')
                .attr('stroke-width', 2);

            node.append('text')
                .text(d => d.id.substring(0, 8))
                .attr('x', 0)
                .attr('y', d => -(10 + (d.weight / maxWeight) * 14))
                .attr('text-anchor', 'middle')
                .attr('fill', '#8b949e')
                .attr('font-size', '10px')
                .attr('font-family', 'monospace')
                .style('pointer-events', 'none');

            const tooltip = d3.select('#graph-container')
                .append('div')
                .style('position', 'absolute')
                .style('background', '#161b22')
                .style('border', '1px solid #30363d')
                .style('border-radius', '6px')
                .style('padding', '8px 12px')
                .style('font-size', '11px')
                .style('font-family', 'monospace')
                .style('color', '#c9d1d9')
                .style('pointer-events', 'none')
                .style('opacity', 0)
                .style('z-index', 100);

            node.on('mouseover', (event, d) => {
                tooltip.html(d.full + '<br><span style="color:#58a6ff">Weight: ' + d.weight.toLocaleString() + '</span>')
                    .style('left', (event.offsetX + 10) + 'px')
                    .style('top', (event.offsetY - 10) + 'px')
                    .style('opacity', 1);
            }).on('mouseout', () => tooltip.style('opacity', 0));

            simulation.on('tick', () => {
                link.attr('x1', d => d.source.x).attr('y1', d => d.source.y)
                    .attr('x2', d => d.target.x).attr('y2', d => d.target.y);
                node.attr('transform', d => 'translate(' + d.x + ',' + d.y + ')');
            });
        })();
        </script>
        {{end}}
    </div>
</body>
</html>`
