package stats

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/pablof7z/purplepag.es/storage"
)

var rejectionTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Rejection & REQ Analytics</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: #0a0a0f;
            min-height: 100vh;
            padding: 2rem;
            color: #e4e4e7;
            position: relative;
            overflow-x: hidden;
        }

        body::before {
            content: '';
            position: fixed;
            top: -50%;
            left: -50%;
            width: 200%;
            height: 200%;
            background: radial-gradient(circle at 30% 20%, rgba(139, 92, 246, 0.08) 0%, transparent 50%),
                        radial-gradient(circle at 70% 80%, rgba(217, 70, 239, 0.06) 0%, transparent 50%);
            animation: drift 30s ease-in-out infinite;
            pointer-events: none;
        }

        @keyframes drift {
            0%, 100% { transform: translate(0, 0) rotate(0deg); }
            33% { transform: translate(-5%, 5%) rotate(5deg); }
            66% { transform: translate(5%, -5%) rotate(-5deg); }
        }

        @keyframes shimmer {
            0%, 100% { background-position: 0% 50%; }
            50% { background-position: 100% 50%; }
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            position: relative;
            z-index: 1;
        }

        .back-link {
            display: inline-block;
            margin-bottom: 2rem;
            color: #a78bfa;
            text-decoration: none;
            font-weight: 500;
            transition: color 0.2s;
        }

        .back-link:hover {
            color: #c4b5fd;
        }

        header {
            margin-bottom: 3rem;
            text-align: center;
        }

        h1 {
            font-size: 3rem;
            font-weight: 700;
            margin-bottom: 0.5rem;
            background: linear-gradient(135deg, #a78bfa 0%, #e879f9 50%, #a78bfa 100%);
            background-size: 200% 100%;
            background-clip: text;
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            animation: shimmer 8s ease-in-out infinite;
            letter-spacing: -0.02em;
        }

        .subtitle {
            font-size: 1rem;
            font-weight: 500;
            color: #a1a1aa;
            text-transform: uppercase;
            letter-spacing: 0.15em;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1.5rem;
            margin-bottom: 3rem;
        }

        .stat-card {
            background: linear-gradient(135deg, rgba(139, 92, 246, 0.05) 0%, rgba(217, 70, 239, 0.02) 100%);
            border: 1px solid rgba(167, 139, 250, 0.15);
            border-radius: 24px;
            padding: 2rem;
            text-align: center;
            position: relative;
            overflow: hidden;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
        }

        .stat-card::before {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            height: 1px;
            background: linear-gradient(90deg, transparent, rgba(167, 139, 250, 0.4), transparent);
            opacity: 0;
            transition: opacity 0.3s;
        }

        .stat-card:hover {
            border-color: rgba(167, 139, 250, 0.3);
            transform: translateY(-2px);
            box-shadow: 0 20px 40px rgba(139, 92, 246, 0.1);
        }

        .stat-card:hover::before {
            opacity: 1;
        }

        .stat-label {
            font-size: 0.75rem;
            font-weight: 600;
            color: #a1a1aa;
            text-transform: uppercase;
            letter-spacing: 0.1em;
            margin-bottom: 1rem;
        }

        .stat-value {
            font-size: 2.5rem;
            font-weight: 700;
            line-height: 1;
            background: linear-gradient(135deg, #e4e4e7 0%, #a1a1aa 100%);
            background-clip: text;
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .section {
            background: linear-gradient(135deg, rgba(139, 92, 246, 0.03) 0%, rgba(217, 70, 239, 0.01) 100%);
            border: 1px solid rgba(167, 139, 250, 0.1);
            border-radius: 24px;
            padding: 2.5rem;
            margin-bottom: 2rem;
        }

        h2 {
            font-size: 1.5rem;
            font-weight: 700;
            margin-bottom: 2rem;
            color: #e4e4e7;
            letter-spacing: -0.01em;
        }

        table {
            width: 100%;
            border-collapse: collapse;
        }

        th {
            text-align: left;
            padding: 0.75rem 1rem;
            font-size: 0.75rem;
            font-weight: 600;
            color: #a1a1aa;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            border-bottom: 1px solid rgba(167, 139, 250, 0.1);
        }

        td {
            padding: 0.75rem 1rem;
            font-size: 0.9rem;
            border-bottom: 1px solid rgba(167, 139, 250, 0.05);
        }

        tr:hover {
            background: rgba(167, 139, 250, 0.03);
        }

        .kind-badge {
            display: inline-block;
            padding: 0.25rem 0.75rem;
            background: rgba(167, 139, 250, 0.15);
            border-radius: 12px;
            font-family: 'SF Mono', monospace;
            font-size: 0.85rem;
            color: #a78bfa;
        }

        .pubkey {
            font-family: 'SF Mono', monospace;
            font-size: 0.8rem;
            color: #71717a;
        }

        .count {
            font-weight: 600;
            font-variant-numeric: tabular-nums;
            color: #e4e4e7;
        }

        .time-ago {
            color: #71717a;
            font-size: 0.85rem;
        }

        .empty-state {
            text-align: center;
            padding: 3rem;
            color: #71717a;
        }

        .daily-stats {
            display: grid;
            gap: 0.5rem;
        }

        .daily-row {
            display: flex;
            align-items: center;
            gap: 1rem;
            padding: 0.5rem 0;
            border-bottom: 1px solid rgba(167, 139, 250, 0.05);
        }

        .daily-date {
            font-size: 0.85rem;
            color: #a1a1aa;
            min-width: 100px;
        }

        .daily-kinds {
            display: flex;
            flex-wrap: wrap;
            gap: 0.5rem;
        }

        .daily-kind {
            display: inline-flex;
            align-items: center;
            gap: 0.25rem;
            padding: 0.2rem 0.5rem;
            background: rgba(167, 139, 250, 0.1);
            border-radius: 8px;
            font-size: 0.8rem;
        }

        .daily-kind .kind-num {
            color: #a78bfa;
        }

        .daily-kind .kind-count {
            color: #71717a;
        }

        @media (max-width: 768px) {
            body {
                padding: 1.5rem;
            }

            h1 {
                font-size: 2rem;
            }

            .stats-grid {
                grid-template-columns: repeat(2, 1fr);
            }

            .section {
                padding: 1.5rem;
            }

            .stat-value {
                font-size: 1.75rem;
            }

            table {
                font-size: 0.8rem;
            }

            th, td {
                padding: 0.5rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats" class="back-link">← Back to Stats</a>

        <header>
            <h1>Rejection & REQ Analytics</h1>
            <p class="subtitle">Tracking rejected events and REQ patterns</p>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Rejected Events</div>
                <div class="stat-value">{{.RejectedEventTotal}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Rejected Kinds</div>
                <div class="stat-value">{{.RejectedEventKinds}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Unique Pubkeys</div>
                <div class="stat-value">{{.RejectedEventPubkeys}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Rejected REQs</div>
                <div class="stat-value">{{.RejectedREQTotal}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Unsupported REQ Kinds</div>
                <div class="stat-value">{{.RejectedREQKinds}}</div>
            </div>
        </div>

        <div class="section">
            <h2>🚫 Rejected Events by Kind</h2>
            {{if .RejectedEventsByKind}}
            <table>
                <thead>
                    <tr>
                        <th>Kind</th>
                        <th>Total Rejected</th>
                        <th>Unique Pubkeys</th>
                        <th>Last Seen</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .RejectedEventsByKind}}
                    <tr>
                        <td><span class="kind-badge">{{.Kind}}</span></td>
                        <td class="count">{{.TotalCount}}</td>
                        <td>{{.UniquePubkeys}}</td>
                        <td class="time-ago">{{.LastSeenAgo}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
            {{else}}
            <div class="empty-state">No rejected events recorded yet</div>
            {{end}}
        </div>

        <div class="section">
            <h2>👤 Top Rejected Event Senders</h2>
            {{if .RejectedEventStats}}
            <table>
                <thead>
                    <tr>
                        <th>Pubkey</th>
                        <th>Kind</th>
                        <th>Count</th>
                        <th>Last Seen</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .RejectedEventStats}}
                    <tr>
                        <td class="pubkey">{{.PubkeyShort}}</td>
                        <td><span class="kind-badge">{{.Kind}}</span></td>
                        <td class="count">{{.Count}}</td>
                        <td class="time-ago">{{.LastSeenAgo}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
            {{else}}
            <div class="empty-state">No rejected events recorded yet</div>
            {{end}}
        </div>

        <div class="section">
            <h2>🔍 Rejected REQs (Unsupported Kinds)</h2>
            {{if .RejectedREQStats}}
            <table>
                <thead>
                    <tr>
                        <th>Kind</th>
                        <th>REQ Count</th>
                        <th>Last Seen</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .RejectedREQStats}}
                    <tr>
                        <td><span class="kind-badge">{{.Kind}}</span></td>
                        <td class="count">{{.Count}}</td>
                        <td class="time-ago">{{.LastSeenAgo}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
            {{else}}
            <div class="empty-state">No rejected REQs recorded yet</div>
            {{end}}
        </div>

        <div class="section">
            <h2>📊 All REQ Kinds (Top 50)</h2>
            {{if .REQKindStats}}
            <table>
                <thead>
                    <tr>
                        <th>Kind</th>
                        <th>Total REQs</th>
                        <th>Last Request</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .REQKindStats}}
                    <tr>
                        <td><span class="kind-badge">{{.Kind}}</span></td>
                        <td class="count">{{.TotalRequests}}</td>
                        <td class="time-ago">{{.LastRequestAgo}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
            {{else}}
            <div class="empty-state">No REQ stats recorded yet</div>
            {{end}}
        </div>

        <div class="section">
            <h2>📅 REQ Kinds by Day (Last 7 Days)</h2>
            {{if .REQKindDaily}}
            <div class="daily-stats">
                {{range .REQKindDaily}}
                <div class="daily-row">
                    <span class="daily-date">{{.Date}}</span>
                    <div class="daily-kinds">
                        {{range .Kinds}}
                        <span class="daily-kind">
                            <span class="kind-num">k{{.Kind}}</span>
                            <span class="kind-count">×{{.Count}}</span>
                        </span>
                        {{end}}
                    </div>
                </div>
                {{end}}
            </div>
            {{else}}
            <div class="empty-state">No daily REQ stats recorded yet</div>
            {{end}}
        </div>
    </div>
</body>
</html>`

type RejectionHandler struct {
	storage *storage.Storage
}

func NewRejectionHandler(store *storage.Storage) *RejectionHandler {
	return &RejectionHandler{storage: store}
}

type RejectedEventStatView struct {
	Kind        int
	Pubkey      string
	PubkeyShort string
	Count       int64
	LastSeenAgo string
}

type RejectedKindSummaryView struct {
	Kind          int
	TotalCount    int64
	UniquePubkeys int64
	LastSeenAgo   string
}

type RejectedREQStatView struct {
	Kind        int
	Count       int64
	LastSeenAgo string
}

type REQKindStatView struct {
	Kind           int
	TotalRequests  int64
	LastRequestAgo string
}

type DailyKindView struct {
	Kind  int
	Count int64
}

type DailyStatsView struct {
	Date  string
	Kinds []DailyKindView
}

type RejectionPageData struct {
	RejectedEventTotal   int64
	RejectedEventKinds   int64
	RejectedEventPubkeys int64
	RejectedREQTotal     int64
	RejectedREQKinds     int64

	RejectedEventsByKind []RejectedKindSummaryView
	RejectedEventStats   []RejectedEventStatView
	RejectedREQStats     []RejectedREQStatView
	REQKindStats         []REQKindStatView
	REQKindDaily         []DailyStatsView
}

func (h *RejectionHandler) HandleRejectionStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		now := time.Now()

		// Get totals
		rejectedEventTotal, rejectedEventKinds, rejectedEventPubkeys, _ := h.storage.GetRejectedEventTotals(ctx)
		rejectedREQTotal, rejectedREQKinds, _ := h.storage.GetRejectedREQTotals(ctx)

		// Get rejected events by kind
		rejectedByKind, _ := h.storage.GetRejectedEventsByKind(ctx, 50)
		rejectedByKindViews := make([]RejectedKindSummaryView, 0, len(rejectedByKind))
		for _, r := range rejectedByKind {
			rejectedByKindViews = append(rejectedByKindViews, RejectedKindSummaryView{
				Kind:          r.Kind,
				TotalCount:    r.TotalCount,
				UniquePubkeys: r.UniquePubkeys,
				LastSeenAgo:   formatTimeAgo(now.Sub(r.LastSeen)),
			})
		}

		// Get rejected events stats (pubkey + kind)
		rejectedEventStats, _ := h.storage.GetRejectedEventStats(ctx, 100)
		rejectedEventViews := make([]RejectedEventStatView, 0, len(rejectedEventStats))
		for _, r := range rejectedEventStats {
			short := r.Pubkey
			if len(short) > 16 {
				short = short[:8] + "..." + short[len(short)-8:]
			}
			rejectedEventViews = append(rejectedEventViews, RejectedEventStatView{
				Kind:        r.Kind,
				Pubkey:      r.Pubkey,
				PubkeyShort: short,
				Count:       r.Count,
				LastSeenAgo: formatTimeAgo(now.Sub(r.LastSeen)),
			})
		}

		// Get rejected REQ stats
		rejectedREQStats, _ := h.storage.GetRejectedREQStats(ctx, 50)
		rejectedREQViews := make([]RejectedREQStatView, 0, len(rejectedREQStats))
		for _, r := range rejectedREQStats {
			rejectedREQViews = append(rejectedREQViews, RejectedREQStatView{
				Kind:        r.Kind,
				Count:       r.Count,
				LastSeenAgo: formatTimeAgo(now.Sub(r.LastSeen)),
			})
		}

		// Get all REQ kind stats
		reqKindStats, _ := h.storage.GetREQKindStats(ctx, 50)
		reqKindViews := make([]REQKindStatView, 0, len(reqKindStats))
		for _, r := range reqKindStats {
			reqKindViews = append(reqKindViews, REQKindStatView{
				Kind:           r.Kind,
				TotalRequests:  r.TotalRequests,
				LastRequestAgo: formatTimeAgo(now.Sub(r.LastRequest)),
			})
		}

		// Get daily stats (last 7 days)
		dailyStats, _ := h.storage.GetREQKindDailyStats(ctx, 7, nil)
		dailyByDate := make(map[string][]DailyKindView)
		for _, s := range dailyStats {
			dailyByDate[s.Date] = append(dailyByDate[s.Date], DailyKindView{
				Kind:  s.Kind,
				Count: s.RequestCount,
			})
		}

		// Convert to ordered list
		dailyViews := make([]DailyStatsView, 0)
		seenDates := make(map[string]bool)
		for _, s := range dailyStats {
			if !seenDates[s.Date] {
				seenDates[s.Date] = true
				dailyViews = append(dailyViews, DailyStatsView{
					Date:  s.Date,
					Kinds: dailyByDate[s.Date],
				})
			}
		}

		data := RejectionPageData{
			RejectedEventTotal:   rejectedEventTotal,
			RejectedEventKinds:   rejectedEventKinds,
			RejectedEventPubkeys: rejectedEventPubkeys,
			RejectedREQTotal:     rejectedREQTotal,
			RejectedREQKinds:     rejectedREQKinds,
			RejectedEventsByKind: rejectedByKindViews,
			RejectedEventStats:   rejectedEventViews,
			RejectedREQStats:     rejectedREQViews,
			REQKindStats:         reqKindViews,
			REQKindDaily:         dailyViews,
		}

		tmpl, err := template.New("rejections").Parse(rejectionTemplate)
		if err != nil {
			http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}
