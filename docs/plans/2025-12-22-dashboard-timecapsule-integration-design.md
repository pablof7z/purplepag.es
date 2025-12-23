# Dashboard & Time Capsule Integration Design

**Date:** 2025-12-22
**Goal:** Integrate time capsule stats into the dashboard and unify their visual themes.

## Overview

Integrate time capsule statistics into the relay dashboard and apply consistent GitHub Dark theming across both pages. This creates a unified analytics experience while maintaining the distinct purposes of each page.

## Current State

### Time Capsule (`/timecapsule`)
- Tracks user profile/follow/relay changes over time
- Shows 2 main stats: Total Archived Versions & Users Tracked
- Uses purple/lavender gradient theme (#a78bfa, background #0a0a0f)

### Dashboard (`/stats/dashboard`)
- Shows relay analytics (REQs, IPs, events served)
- Uses GitHub Dark blue theme (#58a6ff, background #0d1117)
- Has 3 stat cards at top + charts below

## Changes

### 1. Dashboard Integration

**Stat Cards:**
- Add two new cards to existing summary row (5 total cards)
- Card 4: "Users Tracked" - links to `/timecapsule`
- Card 5: "Archived Versions" - links to `/timecapsule`
- Update grid: `grid-template-columns: repeat(5, 1fr);`

**Backend:**
- Add to `DashboardData` struct:
  ```go
  UsersTracked      int
  ArchivedVersions  int
  ```
- Fetch values:
  - `storage.CountTrackedUsers()` - count distinct pubkeys in event_archive
  - `storage.CountArchivedEvents()` - total archived events
- Pass to template

### 2. Time Capsule Theme Overhaul

**Color Palette:**
- Background: `#0a0a0f` → `#0d1117`
- Primary: `#a78bfa` (purple) → `#58a6ff` (blue)
- Text: `#e4e4e7` → `#c9d1d9`
- Borders: Purple rgba → `#21262d`
- Green (additions): `#3fb950`
- Red (deletions): `#f85149`

**Typography:**
- Apply monospace: `'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace`
- Match dashboard font sizing

**Components:**
- Remove gradient headers → solid blue
- Update all borders to GitHub style
- Blue accents on hover states
- GitHub-style search input

### 3. Implementation Steps

1. **Add Storage Method** (if needed)
   - Implement `CountTrackedUsers()` in storage layer
   - Query: `SELECT COUNT(DISTINCT pubkey) FROM event_archive`

2. **Update Dashboard Handler**
   - Fetch time capsule stats
   - Add to data struct
   - Pass to template

3. **Update Dashboard Template**
   - Modify grid CSS to 5 columns
   - Add two new stat cards with links
   - Apply hover styles

4. **Update Time Capsule Template**
   - Replace all colors in `<style>` section
   - Update font declarations
   - Remove gradients
   - Test all components

### 4. Files Modified

- `stats/dashboard.go` - Handler + template
- `pages/timecapsule.go` - Theme CSS
- `storage/storage.go` - Possibly add `CountTrackedUsers()`

## User Flow

1. User visits `/stats/dashboard`
2. Sees 5 stat cards including time capsule metrics
3. Clicks either time capsule card → navigates to `/timecapsule`
4. Time capsule has consistent GitHub Dark theme

## Testing

- Verify stat values are accurate
- Test links from dashboard cards
- Check responsive behavior
- Ensure theme consistency

## Architecture

- No new routes needed (already registered in `main.go`)
- No database schema changes
- Reuse existing storage methods
- Pure Go templates with embedded CSS (no JS framework)
