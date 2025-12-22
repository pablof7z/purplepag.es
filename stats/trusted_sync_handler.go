package stats

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/pablof7z/purplepag.es/storage"
)

var trustedSyncTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Trusted Sync Stats</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 2rem;
            color: #fff;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
        }

        header {
            text-align: center;
            margin-bottom: 3rem;
        }

        h1 {
            font-size: 3rem;
            margin-bottom: 0.5rem;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }

        .subtitle {
            font-size: 1.2rem;
            opacity: 0.9;
        }

        .back-link {
            display: inline-block;
            margin-bottom: 2rem;
            color: #fff;
            text-decoration: none;
            opacity: 0.8;
            transition: opacity 0.2s;
        }

        .back-link:hover {
            opacity: 1;
        }

        .summary-cards {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1.5rem;
            margin-bottom: 2rem;
        }

        .card {
            background: rgba(255, 255, 255, 0.15);
            backdrop-filter: blur(10px);
            border-radius: 16px;
            padding: 1.5rem;
            text-align: center;
            border: 1px solid rgba(255, 255, 255, 0.2);
        }

        .card-value {
            font-size: 2.5rem;
            font-weight: 700;
            margin-bottom: 0.5rem;
        }

        .card-label {
            font-size: 0.9rem;
            opacity: 0.8;
            text-transform: uppercase;
            letter-spacing: 1px;
        }

        h2 {
            font-size: 1.5rem;
            margin-bottom: 1rem;
            margin-top: 2rem;
        }

        .table-container {
            background: rgba(255, 255, 255, 0.15);
            backdrop-filter: blur(10px);
            border-radius: 16px;
            padding: 2rem;
            border: 1px solid rgba(255, 255, 255, 0.2);
            overflow-x: auto;
            margin-bottom: 2rem;
        }

        table {
            width: 100%;
            border-collapse: separate;
            border-spacing: 0;
        }

        thead th {
            background: rgba(255, 255, 255, 0.1);
            padding: 1rem;
            text-align: left;
            font-weight: 600;
            text-transform: uppercase;
            font-size: 0.85rem;
            letter-spacing: 0.5px;
            border-bottom: 2px solid rgba(255, 255, 255, 0.2);
        }

        thead th:first-child {
            border-top-left-radius: 8px;
        }

        thead th:last-child {
            border-top-right-radius: 8px;
        }

        tbody tr {
            transition: background 0.2s;
        }

        tbody tr:hover {
            background: rgba(255, 255, 255, 0.1);
        }

        tbody td {
            padding: 1rem;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .relay-url {
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 0.9rem;
            max-width: 400px;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }

        .relay-url a {
            color: #fff;
            text-decoration: none;
        }

        .relay-url a:hover {
            text-decoration: underline;
        }

        .pubkey {
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 0.85rem;
        }

        .events-count {
            font-weight: 600;
            color: #90EE90;
        }

        .time-ago {
            opacity: 0.8;
            font-size: 0.9rem;
        }

        .no-data {
            text-align: center;
            padding: 3rem;
            opacity: 0.8;
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats" class="back-link">← Back to Stats</a>

        <header>
            <h1>Trusted Sync Stats</h1>
            <div class="subtitle">Events fetched from trusted users' write relays</div>
        </header>

        <div class="summary-cards">
            <div class="card">
                <div class="card-value">{{.TotalEvents}}</div>
                <div class="card-label">Total Events</div>
            </div>
            <div class="card">
                <div class="card-value">{{.TotalPubkeys}}</div>
                <div class="card-label">Unique Pubkeys</div>
            </div>
            <div class="card">
                <div class="card-value">{{.TotalRelays}}</div>
                <div class="card-label">Relays Used</div>
            </div>
        </div>

        <h2>By Relay</h2>
        {{if .RelayStats}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Relay URL</th>
                        <th>Events Fetched</th>
                        <th>Unique Pubkeys</th>
                        <th>Last Sync</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .RelayStats}}
                    <tr>
                        <td class="relay-url"><a href="{{.RelayURL}}" target="_blank" rel="noopener">{{.RelayURL}}</a></td>
                        <td class="events-count">{{.TotalEvents}}</td>
                        <td>{{.UniquePubkeys}}</td>
                        <td class="time-ago">{{.LastSyncAgo}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="table-container">
            <div class="no-data">
                <p>No sync data yet. The trusted syncer will start collecting data after the trust analyzer runs.</p>
            </div>
        </div>
        {{end}}

        <h2>Top Pubkeys</h2>
        {{if .PubkeyStats}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Pubkey</th>
                        <th>Events Fetched</th>
                        <th>Relays</th>
                        <th>Last Sync</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .PubkeyStats}}
                    <tr>
                        <td class="pubkey">{{.PubkeyShort}}</td>
                        <td class="events-count">{{.TotalEvents}}</td>
                        <td>{{.RelayCount}}</td>
                        <td class="time-ago">{{.LastSyncAgo}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="table-container">
            <div class="no-data">
                <p>No pubkey data yet.</p>
            </div>
        </div>
        {{end}}
    </div>
</body>
</html>`

type TrustedSyncRelayInfo struct {
	RelayURL      string
	TotalEvents   int64
	UniquePubkeys int64
	LastSyncAgo   string
}

type TrustedSyncPubkeyInfo struct {
	PubkeyShort string
	TotalEvents int64
	RelayCount  int64
	LastSyncAgo string
}

type TrustedSyncPageData struct {
	TotalEvents  int64
	TotalPubkeys int64
	TotalRelays  int64
	RelayStats   []TrustedSyncRelayInfo
	PubkeyStats  []TrustedSyncPubkeyInfo
}

type TrustedSyncHandler struct {
	storage *storage.Storage
}

func NewTrustedSyncHandler(store *storage.Storage) *TrustedSyncHandler {
	return &TrustedSyncHandler{storage: store}
}

func (h *TrustedSyncHandler) HandleTrustedSyncStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		totalEvents, totalPubkeys, totalRelays, err := h.storage.GetTrustedSyncTotalStats(ctx)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		relayStats, err := h.storage.GetTrustedSyncRelayStats(ctx)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		pubkeyStats, err := h.storage.GetTrustedSyncPubkeyStats(ctx, 50)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		now := time.Now()

		relayInfos := make([]TrustedSyncRelayInfo, 0, len(relayStats))
		for _, stat := range relayStats {
			relayInfos = append(relayInfos, TrustedSyncRelayInfo{
				RelayURL:      stat.RelayURL,
				TotalEvents:   stat.TotalEvents,
				UniquePubkeys: stat.UniquePubkeys,
				LastSyncAgo:   timeAgo(now, time.Unix(stat.LastSyncAt, 0)),
			})
		}

		pubkeyInfos := make([]TrustedSyncPubkeyInfo, 0, len(pubkeyStats))
		for _, stat := range pubkeyStats {
			short := stat.Pubkey
			if len(short) > 16 {
				short = short[:16] + "..."
			}
			pubkeyInfos = append(pubkeyInfos, TrustedSyncPubkeyInfo{
				PubkeyShort: short,
				TotalEvents: stat.TotalEvents,
				RelayCount:  stat.RelayCount,
				LastSyncAgo: timeAgo(now, time.Unix(stat.LastSyncAt, 0)),
			})
		}

		data := TrustedSyncPageData{
			TotalEvents:  totalEvents,
			TotalPubkeys: totalPubkeys,
			TotalRelays:  totalRelays,
			RelayStats:   relayInfos,
			PubkeyStats:  pubkeyInfos,
		}

		tmpl, err := template.New("trusted_sync").Parse(trustedSyncTemplate)
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(w, data)
	}
}

func timeAgo(now, t time.Time) string {
	if t.IsZero() || t.Unix() == 0 {
		return "Never"
	}

	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "Just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
