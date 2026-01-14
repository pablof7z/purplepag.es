package pages

const rankingsTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Most Followed | purplepag.es</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }

        body {
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: #0a0a0f;
            color: #e4e4e7;
            min-height: 100vh;
            padding: 0;
        }

        .container {
            max-width: 1100px;
            margin: 0 auto;
            padding: 2rem 1.5rem;
        }

        header {
            margin-bottom: 3rem;
            border-bottom: 1px solid rgba(139, 92, 246, 0.2);
            padding-bottom: 2rem;
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 0.75rem;
            margin-bottom: 0.75rem;
        }

        .logo-icon {
            width: 40px;
            height: 40px;
            background: linear-gradient(135deg, #8b5cf6, #6366f1);
            border-radius: 10px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.5rem;
        }

        h1 {
            font-size: 1.75rem;
            font-weight: 700;
            background: linear-gradient(135deg, #a78bfa, #8b5cf6);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .subtitle {
            color: #71717a;
            font-size: 0.95rem;
            margin-top: 0.5rem;
        }

        nav {
            display: flex;
            gap: 0.5rem;
            margin-bottom: 2.5rem;
            background: #18181b;
            padding: 0.5rem;
            border-radius: 12px;
            border: 1px solid #27272a;
        }

        nav a {
            color: #a1a1aa;
            text-decoration: none;
            padding: 0.625rem 1.25rem;
            border-radius: 8px;
            transition: all 0.2s;
            font-size: 0.9rem;
            font-weight: 500;
        }

        nav a:hover {
            background: #27272a;
            color: #e4e4e7;
        }

        .stats {
            background: #18181b;
            border: 1px solid #27272a;
            padding: 1rem 1.5rem;
            border-radius: 10px;
            color: #a1a1aa;
            margin-bottom: 1.5rem;
            font-size: 0.9rem;
        }

        .stats strong {
            color: #8b5cf6;
        }

        .profile-card {
            background: #18181b;
            border: 1px solid #27272a;
            border-radius: 12px;
            padding: 1.25rem;
            margin-bottom: 0.75rem;
            display: grid;
            grid-template-columns: auto auto 1fr auto;
            align-items: center;
            gap: 1.25rem;
            transition: all 0.2s;
        }

        .profile-card:hover {
            border-color: #8b5cf6;
            background: #1f1f23;
        }

        .rank {
            font-size: 1.25rem;
            font-weight: 700;
            color: #52525b;
            min-width: 50px;
            text-align: right;
            font-variant-numeric: tabular-nums;
        }

        .avatar {
            width: 48px;
            height: 48px;
            border-radius: 10px;
            background: linear-gradient(135deg, #8b5cf6, #6366f1);
            display: flex;
            align-items: center;
            justify-content: center;
            color: white;
            font-weight: 600;
            font-size: 1.1rem;
            flex-shrink: 0;
            text-transform: uppercase;
        }

        .avatar img {
            width: 100%;
            height: 100%;
            border-radius: 10px;
            object-fit: cover;
        }

        .profile-info {
            min-width: 0;
        }

        .profile-name {
            font-size: 1rem;
            font-weight: 600;
            margin-bottom: 0.25rem;
            color: #e4e4e7;
            font-family: 'SF Mono', 'Monaco', monospace;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }

        .profile-name a {
            color: inherit;
            text-decoration: none;
            transition: color 0.2s;
        }

        .profile-name a:hover {
            color: #8b5cf6;
        }

        .profile-nip05 {
            color: #8b5cf6;
            font-size: 0.825rem;
            margin-bottom: 0.25rem;
        }

        .profile-about {
            color: #71717a;
            font-size: 0.875rem;
            line-height: 1.4;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }

        .profile-stats {
            text-align: right;
        }

        .follower-count {
            font-size: 1.5rem;
            font-weight: 700;
            color: #8b5cf6;
            font-variant-numeric: tabular-nums;
        }

        .follower-label {
            font-size: 0.75rem;
            color: #52525b;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .pagination {
            display: flex;
            justify-content: center;
            align-items: center;
            gap: 0.75rem;
            margin-top: 3rem;
        }

        .pagination a, .pagination span {
            padding: 0.625rem 1.25rem;
            background: #18181b;
            border: 1px solid #27272a;
            border-radius: 8px;
            text-decoration: none;
            color: #a1a1aa;
            font-weight: 500;
            transition: all 0.2s;
            font-size: 0.9rem;
        }

        .pagination a:hover {
            background: #8b5cf6;
            border-color: #8b5cf6;
            color: white;
        }

        .pagination .current {
            background: #8b5cf6;
            border-color: #8b5cf6;
            color: white;
        }

        .pagination .disabled {
            opacity: 0.3;
            pointer-events: none;
        }

        @media (max-width: 768px) {
            .profile-card {
                grid-template-columns: auto 1fr;
                gap: 1rem;
            }

            .rank {
                grid-column: 1;
                grid-row: 1 / 3;
                text-align: left;
                font-size: 1rem;
            }

            .avatar {
                grid-column: 2;
                grid-row: 1;
            }

            .profile-info {
                grid-column: 1 / 3;
                grid-row: 2;
            }

            .profile-stats {
                grid-column: 2;
                grid-row: 1;
                text-align: right;
            }

            .follower-count {
                font-size: 1.25rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo">
                <div class="logo-icon">🟣</div>
                <div>
                    <h1>purplepag.es</h1>
                    <p class="subtitle">Nostr Profile Rankings & Discovery</p>
                </div>
            </div>
        </header>

        <nav>
            <a href="/rankings">Rankings</a>
            <a href="/stats">Stats</a>
        </nav>

        <div class="stats">
            <strong>{{.Total}}</strong> profiles ranked · Page <strong>{{.Page}}</strong> of <strong>{{.TotalPages}}</strong>
        </div>

        {{range $index, $profile := .Profiles}}
        <div class="profile-card">
            <div class="rank">#{{add 1 (add $index (mul (sub $.Page 1) 50))}}</div>
            <div class="avatar">
                {{if $profile.Picture}}
                    <img src="{{$profile.Picture}}" alt="{{$profile.Name}}">
                {{else}}
                    {{slice $profile.Name 0 1}}
                {{end}}
            </div>
            <div class="profile-info">
                <div class="profile-name">
                    <a href="/profile?pubkey={{$profile.Pubkey}}">
                        {{if $profile.DisplayName}}{{$profile.DisplayName}}{{else}}{{$profile.Name}}{{end}}
                    </a>
                </div>
                {{if $profile.Nip05}}
                <div class="profile-nip05">✓ {{$profile.Nip05}}</div>
                {{end}}
                {{if $profile.About}}
                <div class="profile-about">{{$profile.About}}</div>
                {{end}}
            </div>
            <div class="profile-stats">
                <div class="follower-count">{{$profile.FollowerCount}}</div>
                <div class="follower-label">followers</div>
            </div>
        </div>
        {{end}}

        <div class="pagination">
            {{if .HasPrev}}
                <a href="/rankings?page={{sub .Page 1}}">← Prev</a>
            {{else}}
                <span class="disabled">← Prev</span>
            {{end}}

            <span class="current">{{.Page}}</span>

            {{if .HasNext}}
                <a href="/rankings?page={{add .Page 1}}">Next →</a>
            {{else}}
                <span class="disabled">Next →</span>
            {{end}}
        </div>
    </div>
</body>
</html>`

const profileTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{if .Profile.DisplayName}}{{.Profile.DisplayName}}{{else}}{{.Profile.Name}}{{end}} | purplepag.es</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }

        body {
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: #0a0a0f;
            color: #e4e4e7;
            min-height: 100vh;
            padding: 0;
        }

        .container {
            max-width: 1100px;
            margin: 0 auto;
            padding: 2rem 1.5rem;
        }

        header {
            margin-bottom: 3rem;
            border-bottom: 1px solid rgba(139, 92, 246, 0.2);
            padding-bottom: 2rem;
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 0.75rem;
            margin-bottom: 0.75rem;
        }

        .logo-icon {
            width: 40px;
            height: 40px;
            background: linear-gradient(135deg, #8b5cf6, #6366f1);
            border-radius: 10px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.5rem;
        }

        h1 {
            font-size: 1.75rem;
            font-weight: 700;
            background: linear-gradient(135deg, #a78bfa, #8b5cf6);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .subtitle {
            color: #71717a;
            font-size: 0.95rem;
            margin-top: 0.5rem;
        }

        nav {
            display: flex;
            gap: 0.5rem;
            margin-bottom: 2.5rem;
            background: #18181b;
            padding: 0.5rem;
            border-radius: 12px;
            border: 1px solid #27272a;
        }

        nav a {
            color: #a1a1aa;
            text-decoration: none;
            padding: 0.625rem 1.25rem;
            border-radius: 8px;
            transition: all 0.2s;
            font-size: 0.9rem;
            font-weight: 500;
        }

        nav a:hover {
            background: #27272a;
            color: #e4e4e7;
        }

        .profile-header {
            background: #18181b;
            border: 1px solid #27272a;
            border-radius: 12px;
            padding: 2rem;
            margin-bottom: 1.5rem;
        }

        .profile-main {
            display: flex;
            gap: 2rem;
            align-items: flex-start;
            margin-bottom: 2rem;
        }

        .profile-avatar {
            width: 100px;
            height: 100px;
            border-radius: 12px;
            background: linear-gradient(135deg, #8b5cf6, #6366f1);
            display: flex;
            align-items: center;
            justify-content: center;
            color: white;
            font-weight: 700;
            font-size: 2.5rem;
            flex-shrink: 0;
            text-transform: uppercase;
        }

        .profile-avatar img {
            width: 100%;
            height: 100%;
            border-radius: 12px;
            object-fit: cover;
        }

        .profile-details {
            flex: 1;
        }

        .profile-display-name {
            font-size: 1.75rem;
            font-weight: 700;
            color: #e4e4e7;
            margin-bottom: 0.5rem;
            font-family: 'SF Mono', 'Monaco', monospace;
        }

        .profile-nip05 {
            color: #8b5cf6;
            font-size: 0.95rem;
            margin-bottom: 1rem;
        }

        .profile-about {
            color: #a1a1aa;
            line-height: 1.6;
            margin-bottom: 1.5rem;
        }

        .profile-stats {
            display: flex;
            gap: 3rem;
            margin-top: 1.5rem;
        }

        .stat {
            text-align: left;
        }

        .stat-value {
            font-size: 2rem;
            font-weight: 700;
            color: #8b5cf6;
            font-variant-numeric: tabular-nums;
        }

        .stat-label {
            color: #52525b;
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-top: 0.25rem;
        }

        .profile-pubkey {
            background: #0a0a0f;
            border: 1px solid #27272a;
            padding: 1rem;
            border-radius: 8px;
            font-family: 'SF Mono', 'Monaco', monospace;
            font-size: 0.8rem;
            color: #71717a;
            word-break: break-all;
        }

        .profile-pubkey strong {
            color: #a1a1aa;
        }

        .section {
            background: #18181b;
            border: 1px solid #27272a;
            border-radius: 12px;
            padding: 2rem;
            margin-bottom: 1.5rem;
        }

        .section-title {
            font-size: 1.25rem;
            font-weight: 700;
            color: #e4e4e7;
            margin-bottom: 1.5rem;
        }

        .section-title span {
            color: #52525b;
            font-size: 0.9rem;
            font-weight: 400;
        }

        .profile-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
            gap: 0.75rem;
        }

        .mini-profile {
            display: flex;
            align-items: center;
            gap: 1rem;
            padding: 1rem;
            background: #0a0a0f;
            border: 1px solid #27272a;
            border-radius: 8px;
            transition: all 0.2s;
        }

        .mini-profile:hover {
            border-color: #8b5cf6;
            background: #121217;
        }

        .mini-avatar {
            width: 40px;
            height: 40px;
            border-radius: 8px;
            background: linear-gradient(135deg, #8b5cf6, #6366f1);
            display: flex;
            align-items: center;
            justify-content: center;
            color: white;
            font-weight: 600;
            flex-shrink: 0;
            font-size: 0.95rem;
            text-transform: uppercase;
        }

        .mini-avatar img {
            width: 100%;
            height: 100%;
            border-radius: 8px;
            object-fit: cover;
        }

        .mini-info {
            flex: 1;
            min-width: 0;
        }

        .mini-name {
            font-weight: 600;
            color: #e4e4e7;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            font-family: 'SF Mono', 'Monaco', monospace;
            font-size: 0.9rem;
        }

        .mini-name a {
            color: inherit;
            text-decoration: none;
            transition: color 0.2s;
        }

        .mini-name a:hover {
            color: #8b5cf6;
        }

        @media (max-width: 768px) {
            h1 { font-size: 2rem; }
            .profile-main { flex-direction: column; text-align: center; }
            .profile-stats { justify-content: center; }
            .profile-grid { grid-template-columns: 1fr; }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo">
                <div class="logo-icon">🟣</div>
                <div>
                    <h1>purplepag.es</h1>
                    <p class="subtitle">Nostr Profile Rankings & Discovery</p>
                </div>
            </div>
        </header>

        <nav>
            <a href="/rankings">Rankings</a>
            <a href="/stats">Stats</a>
        </nav>

        <div class="profile-header">
            <div class="profile-main">
                <div class="profile-avatar">
                    {{if .Profile.Picture}}
                        <img src="{{.Profile.Picture}}" alt="{{.Profile.Name}}">
                    {{else}}
                        {{slice .Profile.Name 0 1}}
                    {{end}}
                </div>
                <div class="profile-details">
                    <div class="profile-display-name">
                        {{if .Profile.DisplayName}}{{.Profile.DisplayName}}{{else}}{{.Profile.Name}}{{end}}
                    </div>
                    {{if .Profile.Nip05}}
                    <div class="profile-nip05">✓ {{.Profile.Nip05}}</div>
                    {{end}}
                    {{if .Profile.About}}
                    <div class="profile-about">{{.Profile.About}}</div>
                    {{end}}
                    <div class="profile-stats">
                        <div class="stat">
                            <div class="stat-value">{{.Profile.FollowerCount}}</div>
                            <div class="stat-label">Followers</div>
                        </div>
                        <div class="stat">
                            <div class="stat-value">{{.Profile.FollowingCount}}</div>
                            <div class="stat-label">Following</div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="profile-pubkey">
                <strong>Public Key:</strong> {{.Profile.Pubkey}}
            </div>
        </div>

        {{if .Following}}
        <div class="section">
            <div class="section-title">Following <span>({{len .Following}})</span></div>
            <div class="profile-grid">
                {{range .Following}}
                <div class="mini-profile">
                    <div class="mini-avatar">
                        {{if .Picture}}
                            <img src="{{.Picture}}" alt="{{.Name}}">
                        {{else}}
                            {{slice .Name 0 1}}
                        {{end}}
                    </div>
                    <div class="mini-info">
                        <div class="mini-name">
                            <a href="/profile?pubkey={{.Pubkey}}">
                                {{if .DisplayName}}{{.DisplayName}}{{else}}{{.Name}}{{end}}
                            </a>
                        </div>
                    </div>
                </div>
                {{end}}
            </div>
        </div>
        {{end}}
    </div>
</body>
</html>`
