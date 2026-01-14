package stats

import (
	"context"
	"html/template"
	"net/http"
)

var relaysTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Discovered Relays</title>
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
        .table-container {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            overflow-x: auto;
        }
        table { width: 100%; border-collapse: collapse; }
        thead th {
            padding: 0.5rem;
            text-align: left;
            font-weight: 600;
            text-transform: uppercase;
            font-size: 0.625rem;
            color: #8b949e;
            border-bottom: 1px solid #21262d;
        }
        tbody tr:hover { background: #1c2128; }
        tbody td { padding: 0.5rem; border-bottom: 1px solid #21262d; font-size: 0.75rem; }
        .relay-url a { color: #58a6ff; text-decoration: none; }
        .relay-url a:hover { text-decoration: underline; }
        .events-count { font-weight: 600; font-variant-numeric: tabular-nums; color: #f0f6fc; }
        .no-relays { text-align: center; padding: 2rem; color: #8b949e; }
        @media (max-width: 768px) {
            body { padding: 1rem; }
            thead th, tbody td { padding: 0.375rem; }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats" class="back-link">← Back to Stats</a>

        <header>
            <h1>Discovered Relays</h1>
            <div class="subtitle">{{.TotalCount}} relays discovered from kind:10002 events</div>
        </header>

        {{if .Relays}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Relay URL</th>
                        <th>Pubkeys</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Relays}}
                    <tr>
                        <td class="relay-url"><a href="{{.URL}}" target="_blank" rel="noopener">{{.URL}}</a></td>
                        <td class="events-count">{{.PubkeyCount}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="table-container">
            <div class="no-relays">
                <p>No relays discovered yet. Kind:10002 events will be scanned for relay URLs.</p>
            </div>
        </div>
        {{end}}
    </div>
</body>
</html>`

type RelayInfo struct {
	URL         string
	PubkeyCount int64
}

type RelaysPageData struct {
	TotalCount int
	Relays     []RelayInfo
}

func (s *Stats) HandleRelays() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		relays, err := s.storage.GetRelayStats(ctx)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		relayInfos := make([]RelayInfo, 0, len(relays))

		for _, relay := range relays {
			relayInfos = append(relayInfos, RelayInfo{
				URL:         relay.URL,
				PubkeyCount: relay.PubkeyCount,
			})
		}

		data := RelaysPageData{
			TotalCount: len(relayInfos),
			Relays:     relayInfos,
		}

		tmpl, err := template.New("relays").Parse(relaysTemplate)
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
