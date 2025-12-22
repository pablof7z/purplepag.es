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
            font-family: -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, sans-serif;
            background: #0a0a0f;
            min-height: 100vh;
            padding: 2rem;
            color: #e4e4e7;
        }
        body::before {
            content: '';
            position: fixed;
            top: -50%; left: -50%;
            width: 200%; height: 200%;
            background: radial-gradient(circle at 30% 20%, rgba(139, 92, 246, 0.08) 0%, transparent 50%),
                        radial-gradient(circle at 70% 80%, rgba(217, 70, 239, 0.06) 0%, transparent 50%);
            pointer-events: none;
        }
        .container { max-width: 1400px; margin: 0 auto; position: relative; z-index: 1; }
        header { margin-bottom: 3rem; text-align: center; }
        h1 {
            font-size: 2.5rem; font-weight: 700; margin-bottom: 0.5rem;
            background: linear-gradient(135deg, #a78bfa 0%, #e879f9 50%, #a78bfa 100%);
            background-clip: text; -webkit-background-clip: text; -webkit-text-fill-color: transparent;
        }
        .subtitle { font-size: 1rem; color: #a1a1aa; text-transform: uppercase; letter-spacing: 0.15em; }
        .back-link { display: inline-block; margin-bottom: 1rem; color: #a78bfa; text-decoration: none; }
        .back-link:hover { color: #c4b5fd; }

        .search-box {
            background: rgba(139, 92, 246, 0.05);
            border: 1px solid rgba(167, 139, 250, 0.15);
            border-radius: 16px;
            padding: 1.5rem;
            margin-bottom: 2rem;
        }
        .search-box input {
            width: 100%;
            padding: 1rem;
            font-size: 1rem;
            background: rgba(0, 0, 0, 0.3);
            border: 1px solid rgba(167, 139, 250, 0.2);
            border-radius: 8px;
            color: #e4e4e7;
            font-family: monospace;
        }
        .search-box input:focus { outline: none; border-color: #a78bfa; }
        .search-box button {
            margin-top: 1rem;
            padding: 0.75rem 2rem;
            background: linear-gradient(135deg, #8b5cf6 0%, #d946ef 100%);
            border: none;
            border-radius: 8px;
            color: white;
            font-weight: 600;
            cursor: pointer;
        }

        .result-card {
            background: rgba(139, 92, 246, 0.08);
            border: 1px solid rgba(167, 139, 250, 0.2);
            border-radius: 16px;
            padding: 1.5rem;
            margin-bottom: 2rem;
        }
        .result-card h3 { color: #a78bfa; margin-bottom: 1rem; }
        .result-card .pubkey { font-family: monospace; font-size: 0.9rem; color: #71717a; word-break: break-all; }
        .result-card .stats { display: flex; gap: 2rem; margin-top: 1rem; }
        .result-card .stat { }
        .result-card .stat-label { font-size: 0.75rem; color: #71717a; text-transform: uppercase; }
        .result-card .stat-value { font-size: 1.5rem; font-weight: 700; color: #e4e4e7; }
        .badge { display: inline-block; padding: 0.25rem 0.75rem; border-radius: 999px; font-size: 0.75rem; font-weight: 600; margin-left: 0.5rem; }
        .badge.trusted { background: rgba(34, 197, 94, 0.2); color: #22c55e; }
        .badge.cluster { background: rgba(239, 68, 68, 0.2); color: #ef4444; }

        .section {
            background: rgba(139, 92, 246, 0.03);
            border: 1px solid rgba(167, 139, 250, 0.1);
            border-radius: 24px;
            padding: 2rem;
            margin-bottom: 2rem;
        }
        .section h2 { font-size: 1.25rem; font-weight: 700; margin-bottom: 1.5rem; color: #e4e4e7; }

        .data-table { width: 100%; border-collapse: collapse; }
        .data-table th, .data-table td { padding: 0.75rem 1rem; text-align: left; border-bottom: 1px solid rgba(167, 139, 250, 0.1); }
        .data-table th { color: #71717a; font-weight: 600; font-size: 0.75rem; text-transform: uppercase; }
        .data-table td { font-size: 0.9rem; }
        .data-table .mono { font-family: monospace; color: #a1a1aa; }
        .data-table .num { font-variant-numeric: tabular-nums; color: #a78bfa; font-weight: 600; }

        .cluster-card {
            background: rgba(239, 68, 68, 0.05);
            border: 1px solid rgba(239, 68, 68, 0.2);
            border-radius: 12px;
            padding: 1rem;
            margin-bottom: 1rem;
        }
        .cluster-card .header { display: flex; justify-content: space-between; margin-bottom: 0.5rem; }
        .cluster-card .members { font-family: monospace; font-size: 0.8rem; color: #71717a; }

        .spam-section {
            background: rgba(239, 68, 68, 0.03);
            border: 1px solid rgba(239, 68, 68, 0.15);
        }
        .purge-btn {
            padding: 0.75rem 2rem;
            background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
            border: none;
            border-radius: 8px;
            color: white;
            font-weight: 600;
            cursor: pointer;
            margin-bottom: 1rem;
        }
        .purge-btn:hover { opacity: 0.9; }

        .message {
            background: rgba(34, 197, 94, 0.1);
            border: 1px solid rgba(34, 197, 94, 0.3);
            color: #22c55e;
            padding: 1rem;
            border-radius: 8px;
            margin-bottom: 2rem;
        }

        .stats-row {
            display: flex;
            gap: 2rem;
            margin-bottom: 2rem;
        }
        .stat-box {
            background: rgba(139, 92, 246, 0.05);
            border: 1px solid rgba(167, 139, 250, 0.15);
            border-radius: 16px;
            padding: 1.5rem;
            flex: 1;
        }
        .stat-box .label { font-size: 0.75rem; color: #71717a; text-transform: uppercase; margin-bottom: 0.5rem; }
        .stat-box .value { font-size: 2rem; font-weight: 700; color: #a78bfa; }

        @media (max-width: 768px) {
            body { padding: 1rem; }
            .stats-row { flex-direction: column; gap: 1rem; }
            .data-table { font-size: 0.8rem; }
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
                        <th>Pubkey</th>
                        <th>Name</th>
                        <th>Requests</th>
                        <th>Last Requested</th>
                        <th>Status</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .TopRequested}}
                    <tr>
                        <td class="mono">{{.ShortPubkey}}</td>
                        <td>{{if .Name}}{{.Name}}{{else}}<span style="color:#52525b">—</span>{{end}}</td>
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
            <p style="color: #71717a; margin-bottom: 1rem; font-size: 0.9rem;">
                Nodes represent pubkeys, edges show co-occurrence frequency. Drag to reposition, scroll to zoom.
            </p>
            <div id="graph-container" style="width: 100%; height: 600px; background: rgba(0,0,0,0.3); border-radius: 12px; overflow: hidden;"></div>
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
                .attr('stroke', '#a78bfa')
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
                    return d3.interpolateRgb('#8b5cf6', '#e879f9')(t);
                })
                .attr('stroke', '#1a1a2e')
                .attr('stroke-width', 2);

            node.append('text')
                .text(d => d.id.substring(0, 8))
                .attr('x', 0)
                .attr('y', d => -(10 + (d.weight / maxWeight) * 14))
                .attr('text-anchor', 'middle')
                .attr('fill', '#a1a1aa')
                .attr('font-size', '10px')
                .attr('font-family', 'monospace')
                .style('pointer-events', 'none');

            const tooltip = d3.select('#graph-container')
                .append('div')
                .style('position', 'absolute')
                .style('background', 'rgba(10, 10, 15, 0.95)')
                .style('border', '1px solid rgba(167, 139, 250, 0.3)')
                .style('border-radius', '8px')
                .style('padding', '8px 12px')
                .style('font-size', '12px')
                .style('font-family', 'monospace')
                .style('color', '#e4e4e7')
                .style('pointer-events', 'none')
                .style('opacity', 0)
                .style('z-index', 100);

            node.on('mouseover', (event, d) => {
                tooltip.html(d.full + '<br><span style="color:#a78bfa">Weight: ' + d.weight.toLocaleString() + '</span>')
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
