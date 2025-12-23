package stats

import (
	"context"
	"html/template"
	"net/http"

	"github.com/pablof7z/purplepag.es/storage"
)

type SocialHandler struct {
	storage *storage.Storage
}

func NewSocialHandler(store *storage.Storage) *SocialHandler {
	return &SocialHandler{storage: store}
}

type MutedDisplay struct {
	Pubkey        string
	Name          string
	MuteCount     int64
	FollowerCount int64
	IsSpam        bool
}

type InterestDisplay struct {
	Interest string
	Count    int64
}

type TrendDisplay struct {
	Pubkey    string
	Name      string
	NetChange int64
	Gained    int64
	Lost      int64
}

type SocialPageData struct {
	MuteListCount      int64
	InterestListCount  int64
	CommunityListCount int64
	ContactListCount   int64
	MostMuted          []MutedDisplay
	TopInterests       []InterestDisplay
	Rising             []TrendDisplay
	Falling            []TrendDisplay
}

func (h *SocialHandler) HandleSocial() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		muteCount, interestCount, communityCount, contactCount, _ := h.storage.GetSocialGraphStats(ctx)

		// Get most muted pubkeys
		mutedPubkeys, _ := h.storage.GetMostMutedPubkeys(ctx, 20)
		pubkeyList := make([]string, len(mutedPubkeys))
		for i, m := range mutedPubkeys {
			pubkeyList[i] = m.Pubkey
		}
		names, _ := h.storage.GetProfileNames(ctx, pubkeyList)

		mostMuted := make([]MutedDisplay, len(mutedPubkeys))
		for i, m := range mutedPubkeys {
			mostMuted[i] = MutedDisplay{
				Pubkey:        m.Pubkey,
				Name:          names[m.Pubkey],
				MuteCount:     m.MuteCount,
				FollowerCount: m.FollowerCount,
				IsSpam:        m.FollowerCount < 10,
			}
		}

		// Get top interests
		interests, _ := h.storage.GetInterestRankings(ctx, 20)
		topInterests := make([]InterestDisplay, len(interests))
		for i, interest := range interests {
			topInterests[i] = InterestDisplay{
				Interest: interest.Interest,
				Count:    interest.Count,
			}
		}

		// Get follower trends
		risingRaw, fallingRaw, _ := h.storage.GetFollowerTrends(ctx, 10)

		// Get names for trends
		trendPubkeys := make([]string, 0, len(risingRaw)+len(fallingRaw))
		for _, t := range risingRaw {
			trendPubkeys = append(trendPubkeys, t.Pubkey)
		}
		for _, t := range fallingRaw {
			trendPubkeys = append(trendPubkeys, t.Pubkey)
		}
		trendNames, _ := h.storage.GetProfileNames(ctx, trendPubkeys)

		rising := make([]TrendDisplay, len(risingRaw))
		for i, t := range risingRaw {
			rising[i] = TrendDisplay{
				Pubkey:    t.Pubkey,
				Name:      trendNames[t.Pubkey],
				NetChange: t.NetChange,
				Gained:    t.Gained,
				Lost:      t.Lost,
			}
		}

		falling := make([]TrendDisplay, len(fallingRaw))
		for i, t := range fallingRaw {
			falling[i] = TrendDisplay{
				Pubkey:    t.Pubkey,
				Name:      trendNames[t.Pubkey],
				NetChange: t.NetChange,
				Gained:    t.Gained,
				Lost:      t.Lost,
			}
		}

		data := SocialPageData{
			MuteListCount:      muteCount,
			InterestListCount:  interestCount,
			CommunityListCount: communityCount,
			ContactListCount:   contactCount,
			MostMuted:          mostMuted,
			TopInterests:       topInterests,
			Rising:             rising,
			Falling:            falling,
		}

		tmpl, err := template.New("social").Parse(socialTemplate)
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

var socialTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Social Graph Analytics</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            background: #0d1117;
            min-height: 100vh;
            padding: 2rem;
            color: #c9d1d9;
        }
        .container { max-width: 1200px; margin: 0 auto; }
        header { margin-bottom: 2rem; border-bottom: 1px solid #21262d; padding-bottom: 1rem; }
        .back-link { color: #58a6ff; text-decoration: none; font-size: 0.875rem; }
        .back-link:hover { text-decoration: underline; }
        h1 { font-size: 1.5rem; font-weight: 600; color: #f0f6fc; margin: 0.5rem 0 0.25rem; }
        .subtitle { font-size: 0.875rem; color: #8b949e; }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
        }
        .stat-label {
            font-size: 0.625rem;
            color: #8b949e;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 0.5rem;
        }
        .stat-value {
            font-size: 1.75rem;
            font-weight: 600;
            color: #f0f6fc;
        }
        .section {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            margin-bottom: 1rem;
        }
        .section h2 { font-size: 0.875rem; font-weight: 600; margin-bottom: 1rem; color: #f0f6fc; }
        .item-list { display: flex; flex-direction: column; gap: 0.25rem; }
        .item {
            background: #0d1117;
            border: 1px solid #21262d;
            padding: 0.5rem 0.75rem;
            border-radius: 6px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .item:hover { border-color: #30363d; }
        .item-name { color: #c9d1d9; font-size: 0.75rem; }
        .item-name a { color: #58a6ff; text-decoration: none; }
        .item-name a:hover { text-decoration: underline; }
        .item-stats { display: flex; gap: 1rem; font-size: 0.75rem; }
        .item-count { font-weight: 600; color: #f85149; }
        .item-followers { color: #8b949e; }
        .spam-badge { background: #f8514922; color: #f85149; padding: 0.125rem 0.375rem; border-radius: 4px; font-size: 0.625rem; margin-left: 0.5rem; }
        .controversial-badge { background: #a371f722; color: #a371f7; padding: 0.125rem 0.375rem; border-radius: 4px; font-size: 0.625rem; margin-left: 0.5rem; }
        .interest-count { font-weight: 600; color: #58a6ff; }
        .trend-positive { color: #238636; font-weight: 600; }
        .trend-negative { color: #f85149; font-weight: 600; }
        .trend-detail { color: #8b949e; font-size: 0.625rem; }
        .two-col { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
        @media (max-width: 768px) { .two-col { grid-template-columns: 1fr; } }
        .footer {
            text-align: center;
            margin-top: 2rem;
            padding-top: 1rem;
            border-top: 1px solid #21262d;
            font-size: 0.75rem;
        }
        .footer a { color: #58a6ff; text-decoration: none; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <a href="/stats" class="back-link">← Back to Stats</a>
            <h1>Social Graph Analytics</h1>
            <div class="subtitle">Network dynamics and influence metrics</div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Mute Lists</div>
                <div class="stat-value">{{.MuteListCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Interest Lists</div>
                <div class="stat-value">{{.InterestListCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Community Lists</div>
                <div class="stat-value">{{.CommunityListCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Contact Lists</div>
                <div class="stat-value">{{.ContactListCount}}</div>
            </div>
        </div>

        <div class="section">
            <h2>Most Muted Accounts</h2>
            <div class="item-list">
                {{range .MostMuted}}
                <div class="item">
                    <span class="item-name">
                        <a href="/profile?pubkey={{.Pubkey}}">{{if .Name}}{{.Name}}{{else}}{{slice .Pubkey 0 12}}...{{end}}</a>
                        {{if .IsSpam}}<span class="spam-badge">SPAM</span>{{else}}<span class="controversial-badge">CONTROVERSIAL</span>{{end}}
                    </span>
                    <span class="item-stats">
                        <span class="item-count">{{.MuteCount}} mutes</span>
                        <span class="item-followers">{{.FollowerCount}} followers</span>
                    </span>
                </div>
                {{end}}
            </div>
        </div>

        <div class="section">
            <h2>Top Interests</h2>
            <div class="item-list">
                {{range .TopInterests}}
                <div class="item">
                    <span class="item-name">#{{.Interest}}</span>
                    <span class="interest-count">{{.Count}} users</span>
                </div>
                {{end}}
            </div>
        </div>

        <div class="two-col">
            <div class="section">
                <h2>Rising (Gaining Followers)</h2>
                <div class="item-list">
                    {{range .Rising}}
                    <div class="item">
                        <span class="item-name">
                            <a href="/profile?pubkey={{.Pubkey}}">{{if .Name}}{{.Name}}{{else}}{{slice .Pubkey 0 12}}...{{end}}</a>
                        </span>
                        <span>
                            <span class="trend-positive">+{{.NetChange}}</span>
                            <span class="trend-detail">(+{{.Gained}} / -{{.Lost}})</span>
                        </span>
                    </div>
                    {{end}}
                </div>
            </div>

            <div class="section">
                <h2>Falling (Losing Followers)</h2>
                <div class="item-list">
                    {{range .Falling}}
                    <div class="item">
                        <span class="item-name">
                            <a href="/profile?pubkey={{.Pubkey}}">{{if .Name}}{{.Name}}{{else}}{{slice .Pubkey 0 12}}...{{end}}</a>
                        </span>
                        <span>
                            <span class="trend-negative">{{.NetChange}}</span>
                            <span class="trend-detail">(+{{.Gained}} / -{{.Lost}})</span>
                        </span>
                    </div>
                    {{end}}
                </div>
            </div>
        </div>

        <div class="footer">
            <a href="/stats">Back to main stats</a>
        </div>
    </div>
</body>
</html>`
