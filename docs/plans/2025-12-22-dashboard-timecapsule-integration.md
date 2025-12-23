# Dashboard & Time Capsule Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add time capsule stats to dashboard and unify themes with GitHub Dark styling.

**Architecture:** Add two stat cards to dashboard showing time capsule metrics (users tracked, archived versions) that link to `/timecapsule`. Update time capsule styling to match GitHub Dark theme for visual consistency.

**Tech Stack:** Go 1.21+, html/template, embedded CSS

---

## Task 1: Add Time Capsule Stats to Dashboard Backend

**Files:**
- Modify: `stats/dashboard.go:362-369` (DashboardData struct)
- Modify: `stats/dashboard.go:371-436` (HandleDashboard function)

**Step 1: Add fields to DashboardData struct**

In `stats/dashboard.go`, update the `DashboardData` struct (around line 362):

```go
type DashboardData struct {
	TodayREQs         int64
	TodayUniqueIPs    int64
	TodayEventsServed int64
	DailyStatsJSON    template.JS
	HourlyStatsJSON   template.JS
	TopIPs            []TopIPDisplay
	UsersTracked      int64
	ArchivedVersions  int64
}
```

**Step 2: Fetch time capsule stats in handler**

In `stats/dashboard.go`, in the `HandleDashboard` function (after line 393, after fetching topIPs):

```go
		topIPs, err := h.storage.GetTopIPs(ctx, 20)
		if err != nil {
			topIPs = []storage.TopIP{}
		}

		// Fetch time capsule stats
		archivedVersions, usersTracked, err := h.storage.GetEventHistoryStats(ctx)
		if err != nil {
			archivedVersions = 0
			usersTracked = 0
		}
```

**Step 3: Add fields to data struct initialization**

In `stats/dashboard.go`, update the `DashboardData` initialization (around line 416):

```go
		data := DashboardData{
			TodayREQs:         todayStats.TotalREQs,
			TodayUniqueIPs:    todayStats.UniqueIPs,
			TodayEventsServed: todayStats.EventsServed,
			DailyStatsJSON:    template.JS(dailyStatsJSON),
			HourlyStatsJSON:   template.JS(hourlyStatsJSON),
			TopIPs:            topIPDisplays,
			UsersTracked:      usersTracked,
			ArchivedVersions:  archivedVersions,
		}
```

**Step 4: Verify the changes**

Run: `go build`
Expected: Successful compilation with no errors

**Step 5: Commit**

```bash
git add stats/dashboard.go
git commit -m "Add time capsule stats to dashboard backend"
```

---

## Task 2: Update Dashboard Template - Add Stat Cards

**Files:**
- Modify: `stats/dashboard.go:33-37` (stats-grid CSS)
- Modify: `stats/dashboard.go:138-151` (stat cards HTML)

**Step 1: Update grid CSS to support 5 columns**

In `stats/dashboard.go`, update the `.stats-grid` CSS (around line 33):

```css
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(5, 1fr);
            gap: 1rem;
            margin-bottom: 2rem;
        }
```

**Step 2: Add clickable card styling**

In `stats/dashboard.go`, after the `.stat-card` definition (around line 44), add:

```css
        .stat-card {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
        }
        .stat-card.clickable {
            cursor: pointer;
            text-decoration: none;
            color: inherit;
            display: block;
            transition: border-color 0.2s;
        }
        .stat-card.clickable:hover {
            border-color: #58a6ff;
        }
```

**Step 3: Add two new stat cards**

In `stats/dashboard.go`, after the third stat card (around line 151), add:

```html
            <a href="/timecapsule" class="stat-card clickable">
                <div class="stat-label">Users Tracked</div>
                <div class="stat-value">{{.UsersTracked}}</div>
            </a>
            <a href="/timecapsule" class="stat-card clickable">
                <div class="stat-label">Archived Versions</div>
                <div class="stat-value">{{.ArchivedVersions}}</div>
            </a>
```

**Step 4: Test the dashboard**

Run: `go run . &` (start server)
Visit: `http://localhost:8080/stats/dashboard`
Expected: Dashboard loads with 5 stat cards, last two link to time capsule

**Step 5: Commit**

```bash
git add stats/dashboard.go
git commit -m "Add time capsule stat cards to dashboard UI"
```

---

## Task 3: Update Time Capsule Theme - Colors

**Files:**
- Modify: `pages/timecapsule.go:308-314` (body and container styles)
- Modify: `pages/timecapsule.go:317-336` (header and title styles)
- Modify: `pages/timecapsule.go:338-360` (stat boxes)

**Step 1: Update body background and text colors**

In `pages/timecapsule.go`, update the body style (around line 309):

```css
        body {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            background: #0d1117;
            min-height: 100vh;
            padding: 2rem;
            color: #c9d1d9;
        }
```

**Step 2: Update header and title - remove gradient**

In `pages/timecapsule.go`, update the header and h1 styles (around line 324):

```css
        header {
            margin-bottom: 2rem;
            text-align: center;
            border-bottom: 1px solid #21262d;
            padding-bottom: 1rem;
        }
        h1 {
            font-size: 2rem;
            font-weight: 600;
            margin-bottom: 0.5rem;
            color: #f0f6fc;
        }
        .subtitle {
            color: #8b949e;
            font-size: 0.875rem;
        }
```

**Step 3: Update back link color**

In `pages/timecapsule.go`, update the `.back-link` style (around line 317):

```css
        .back-link {
            display: inline-block;
            margin-bottom: 1.5rem;
            color: #58a6ff;
            text-decoration: none;
            font-size: 0.875rem;
        }
        .back-link:hover {
            text-decoration: underline;
        }
```

**Step 4: Update stat boxes**

In `pages/timecapsule.go`, update the `.stat-box` styles (around line 338):

```css
        .stat-box {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem 2rem;
            text-align: center;
        }
        .stat-box .value {
            font-size: 2rem;
            font-weight: 600;
            color: #58a6ff;
            font-variant-numeric: tabular-nums;
        }
        .stat-box .label {
            font-size: 0.75rem;
            color: #8b949e;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
```

**Step 5: Commit**

```bash
git add pages/timecapsule.go
git commit -m "Update time capsule theme colors to GitHub Dark"
```

---

## Task 4: Update Time Capsule Theme - Components

**Files:**
- Modify: `pages/timecapsule.go:361-392` (search box)
- Modify: `pages/timecapsule.go:393-437` (delta cards)
- Modify: `pages/timecapsule.go:438-497` (change items and actions)

**Step 1: Update search box styling**

In `pages/timecapsule.go`, update the `.search-box` styles (around line 361):

```css
        .search-box {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1.5rem;
            margin-bottom: 2rem;
        }
        .search-box input {
            width: 100%;
            padding: 0.75rem 1rem;
            font-size: 0.875rem;
            background: #0d1117;
            border: 1px solid #30363d;
            border-radius: 6px;
            color: #c9d1d9;
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
        }
        .search-box input:focus {
            outline: none;
            border-color: #58a6ff;
        }
        .search-box input::placeholder {
            color: #8b949e;
        }
        .search-box button {
            margin-top: 0.75rem;
            padding: 0.5rem 1.5rem;
            background: #238636;
            border: none;
            border-radius: 6px;
            color: #ffffff;
            font-weight: 600;
            cursor: pointer;
            font-size: 0.875rem;
            font-family: inherit;
        }
        .search-box button:hover {
            background: #2ea043;
        }
```

**Step 2: Update delta card styling**

In `pages/timecapsule.go`, update the `.delta-card` and related styles (around line 393):

```css
        .delta-card {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1.5rem;
            margin-bottom: 1rem;
        }
        .delta-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 1rem;
            padding-bottom: 0.75rem;
            border-bottom: 1px solid #21262d;
        }
        .delta-name {
            font-weight: 600;
            color: #f0f6fc;
        }
        .delta-pubkey {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            font-size: 0.75rem;
            color: #8b949e;
        }
        .delta-kind {
            display: inline-block;
            padding: 0.25rem 0.75rem;
            background: #388bfd26;
            border: 1px solid #388bfd;
            border-radius: 20px;
            font-size: 0.75rem;
            color: #58a6ff;
            margin-bottom: 0.25rem;
        }
        .delta-time {
            font-size: 0.75rem;
            color: #8b949e;
        }
```

**Step 3: Update change items and field colors**

In `pages/timecapsule.go`, update the change list styles (around line 438):

```css
        .change-list { list-style: none; }
        .change-item {
            padding: 0.5rem 0;
            border-bottom: 1px solid #21262d;
            display: flex;
            align-items: flex-start;
            gap: 0.75rem;
            font-size: 0.875rem;
        }
        .change-item:last-child { border-bottom: none; }
        .change-field {
            min-width: 100px;
            font-weight: 500;
            color: #8b949e;
            font-size: 0.875rem;
        }
        .change-values { flex: 1; }
        .old-value {
            color: #f85149;
            text-decoration: line-through;
            font-size: 0.875rem;
        }
        .new-value {
            color: #3fb950;
            font-size: 0.875rem;
        }
```

**Step 4: Update follow/unfollow action badges**

In `pages/timecapsule.go`, update the follow action styles (around line 463):

```css
        .follow-action {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.25rem 0.75rem;
            border-radius: 6px;
            font-size: 0.8rem;
            margin: 0.25rem;
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
        }
        .follow-action.followed {
            background: rgba(63, 185, 80, 0.15);
            border: 1px solid #3fb950;
            color: #3fb950;
        }
        .follow-action.unfollowed {
            background: rgba(248, 81, 73, 0.15);
            border: 1px solid #f85149;
            color: #f85149;
        }
```

**Step 5: Update relay action badges**

In `pages/timecapsule.go`, update the relay action styles (around line 480):

```css
        .relay-action {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.25rem 0.75rem;
            border-radius: 6px;
            font-size: 0.75rem;
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            margin: 0.25rem;
        }
        .relay-action.added {
            background: rgba(63, 185, 80, 0.15);
            border: 1px solid #3fb950;
            color: #3fb950;
        }
        .relay-action.removed {
            background: rgba(248, 81, 73, 0.15);
            border: 1px solid #f85149;
            color: #f85149;
        }
```

**Step 6: Update section title and empty state**

In `pages/timecapsule.go`, update these styles (around line 498):

```css
        .empty-state {
            text-align: center;
            padding: 3rem;
            color: #8b949e;
            font-size: 0.875rem;
        }
        .section-title {
            font-size: 1.125rem;
            font-weight: 600;
            margin-bottom: 1.5rem;
            color: #f0f6fc;
        }
```

**Step 7: Commit**

```bash
git add pages/timecapsule.go
git commit -m "Update time capsule component styles to GitHub Dark"
```

---

## Task 5: Fix Time Capsule Template Links

**Files:**
- Modify: `pages/timecapsule.go:619` (pubkey link color)

**Step 1: Update pubkey link in template**

In `pages/timecapsule.go`, find the pubkey link in the template (around line 619):

```html
<a href="/timecapsule?pubkey={{.PubKey}}" class="delta-pubkey" style="color: #58a6ff;">{{.PubKeyShort}}</a>
```

Change to:

```html
<a href="/timecapsule?pubkey={{.PubKey}}" class="delta-pubkey" style="color: #58a6ff; text-decoration: none;">{{.PubKeyShort}}</a>
```

**Step 2: Verify all template text colors match**

Review the template section to ensure:
- All text uses GitHub Dark colors
- Links are `#58a6ff`
- Muted text is `#8b949e`
- Primary text is `#c9d1d9` or `#f0f6fc`

**Step 3: Commit**

```bash
git add pages/timecapsule.go
git commit -m "Fix time capsule template link styling"
```

---

## Task 6: Integration Testing

**Files:**
- Test: `stats/dashboard.go` (manual testing)
- Test: `pages/timecapsule.go` (manual testing)

**Step 1: Build and run the server**

Run: `go build && ./purplepag.es`
Expected: Server starts without errors

**Step 2: Test dashboard**

1. Visit: `http://localhost:8080/stats/dashboard`
2. Verify: 5 stat cards visible
3. Verify: "Users Tracked" and "Archived Versions" show numbers
4. Verify: Cards 4 and 5 change border color on hover
5. Click: "Users Tracked" card
6. Verify: Navigates to `/timecapsule`

**Step 3: Test time capsule theme**

1. Visit: `http://localhost:8080/timecapsule`
2. Verify: Background is `#0d1117` (GitHub Dark)
3. Verify: Stat boxes use blue (`#58a6ff`) not purple
4. Verify: Search input has GitHub Dark styling
5. Verify: Delta cards have GitHub Dark borders
6. Verify: All text is readable with new color scheme
7. Verify: Monospace fonts are applied throughout

**Step 4: Test responsive design**

1. Resize browser to mobile width
2. Verify: Dashboard cards stack properly
3. Verify: Time capsule maintains readability

**Step 5: Test navigation flow**

1. Start at `/stats/dashboard`
2. Click "Users Tracked" → lands on `/timecapsule`
3. Click "← Back to Home" → returns to home
4. Navigate back to dashboard
5. Click "Archived Versions" → lands on `/timecapsule`

**Step 6: Document any issues**

If issues found:
- Note exact steps to reproduce
- Screenshot if visual issue
- Fix before final commit

**Step 7: Final commit**

If any fixes were needed:

```bash
git add .
git commit -m "Fix integration issues from testing"
```

---

## Testing Checklist

- [ ] Dashboard shows 5 stat cards
- [ ] Time capsule stats populate correctly
- [ ] Cards link to `/timecapsule`
- [ ] Hover states work on clickable cards
- [ ] Time capsule has GitHub Dark theme
- [ ] All colors match design spec
- [ ] Monospace fonts applied
- [ ] No purple/lavender colors remain
- [ ] Links are blue `#58a6ff`
- [ ] Responsive design works
- [ ] No console errors
- [ ] No Go compilation errors

---

## Rollback Plan

If issues occur:

```bash
# Revert all commits
git log --oneline  # Note the commit before your changes
git reset --hard <commit-hash>

# Or revert specific commits
git revert <commit-hash>
```

---

## Notes

- No database migrations needed
- No new routes required
- All changes are in existing files
- Storage method `GetEventHistoryStats()` already exists
- DRY: Reusing existing storage methods
- YAGNI: No extra features, just requirements
- No backwards compatibility concerns
