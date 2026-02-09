package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"iter"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	nostr2 "fiatjaf.com/nostr"
	nostrEventstore "fiatjaf.com/nostr/eventstore"
	"fiatjaf.com/nostr/khatru"
	"fiatjaf.com/nostr/nip11"
	"fiatjaf.com/nostr/nip45/hyperloglog"
	"github.com/fiatjaf/eventstore"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip77"
	"github.com/pablof7z/purplepag.es/analytics"
	"github.com/pablof7z/purplepag.es/config"
	"github.com/pablof7z/purplepag.es/pages"
	relay2 "github.com/pablof7z/purplepag.es/relay"
	"github.com/pablof7z/purplepag.es/stats"
	"github.com/pablof7z/purplepag.es/storage"
	"github.com/pablof7z/purplepag.es/sync"
)

func main() {
	// Check for subcommands before parsing flags
	if len(os.Args) > 1 && os.Args[1] == "sync" {
		runSyncCommand(os.Args[2:])
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "analytics" {
		runAnalyticsWorker()
		return
	}

	port := flag.Int("port", 0, "Override port from config (use 9999 for sync-only test mode)")
	importFile := flag.String("import", "", "Import events from JSONL file and exit")
	flag.Parse()

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *port != 0 {
		cfg.Server.Port = *port
	}

	testMode := cfg.Server.Port == 9999

	var store *storage.Storage
	if *importFile != "" {
		store, err = storage.NewForImport(cfg.Storage.Path)
	} else {
		store, err = storage.New(cfg.Storage.Path, *cfg.Storage.ArchiveEnabled)
	}
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	if *importFile != "" {
		if err := importEventsFromJSONL(store, *importFile); err != nil {
			log.Fatalf("Failed to import events: %v", err)
		}
		log.Println("Import completed successfully")
		os.Exit(0)
	}

	if err := store.InitAnalyticsSchema(); err != nil {
		log.Fatalf("Failed to initialize analytics schema: %v", err)
	}

	if err := store.InitDailyStatsSchema(); err != nil {
		log.Fatalf("Failed to initialize daily stats schema: %v", err)
	}

	if err := store.InitEventHistorySchema(); err != nil {
		log.Fatalf("Failed to initialize event history schema: %v", err)
	}

	if err := store.InitStorageStatsSchema(); err != nil {
		log.Fatalf("Failed to initialize storage stats schema: %v", err)
	}
	if err := store.InitCacheSchema(); err != nil {
		log.Fatalf("Failed to initialize cache schema: %v", err)
	}

	statsTracker := stats.New(store)
	analyticsTracker := analytics.NewTracker(store)
	clusterDetector := analytics.NewClusterDetector(store)
	trustAnalyzer := analytics.NewTrustAnalyzer(store, clusterDetector, 10)
	syncQueue := relay2.NewSyncQueue(store, cfg.SyncKinds)

	relay := khatru.NewRelay()

	relay.Info.Name = cfg.Relay.Name
	relay.Info.Description = cfg.Relay.Description
	if cfg.Relay.Pubkey != "" {
		pk, err := nostr2.PubKeyFromHex(cfg.Relay.Pubkey)
		if err != nil {
			log.Printf("Warning: invalid relay pubkey %q: %v", cfg.Relay.Pubkey, err)
		} else {
			relay.Info.PubKey = &pk
		}
	}
	relay.Info.Contact = cfg.Relay.Contact
	relay.Info.Icon = cfg.Relay.Icon
	relay.Info.AddSupportedNIPs(cfg.Relay.SupportedNIPs)
	relay.Info.Software = cfg.Relay.Software
	relay.Info.Version = cfg.Relay.Version
	relay.Info.Limitation = &nip11.RelayLimitationDocument{
		MaxSubscriptions: cfg.Limits.MaxSubscriptions,
		MaxLimit:         cfg.Limits.MaxLimit,
		MaxEventTags:     cfg.Limits.MaxEventTags,
		MaxContentLength: cfg.Limits.MaxContentLength,
	}

	relay.OnEvent = func(ctx context.Context, event nostr2.Event) (bool, string) {
		kind := int(event.Kind)
		if !cfg.IsKindAllowed(kind) {
			statsTracker.RecordEventRejectedForKind(ctx, kind, event.PubKey.Hex())
			return true, fmt.Sprintf("kind %d is not allowed", kind)
		}
		if len(event.Tags) > cfg.Limits.MaxEventTags {
			statsTracker.RecordEventRejected()
			return true, fmt.Sprintf("too many tags: %d (max %d)", len(event.Tags), cfg.Limits.MaxEventTags)
		}
		if len(event.Content) > cfg.Limits.MaxContentLength {
			statsTracker.RecordEventRejected()
			return true, fmt.Sprintf("content too long: %d (max %d)", len(event.Content), cfg.Limits.MaxContentLength)
		}
		return false, ""
	}

	relay.OnRequest = func(ctx context.Context, filter nostr2.Filter) (bool, string) {
		if filter.Limit > cfg.Limits.MaxLimit {
			return true, fmt.Sprintf("limit too high: %d (max %d)", filter.Limit, cfg.Limits.MaxLimit)
		}
		if len(filter.Kinds) == 0 {
			return true, "filters must specify at least one kind"
		}

		ip := khatru.GetIP(ctx)
		eventsServed, err := store.GetEventsServedLast24Hours(ctx, ip)
		if err != nil {
			return false, ""
		}
		if eventsServed < int64(cfg.Limits.EventsPerDayLimit) {
			return false, ""
		}
		authedPubkey, ok := khatru.GetAuthed(ctx)
		if !ok {
			return true, "auth-required: rate limit exceeded"
		}
		trustedFollowers, err := trustAnalyzer.GetTrustedFollowerCount(ctx, authedPubkey.Hex())
		if err != nil {
			return true, "error checking trusted follower count"
		}
		if trustedFollowers < cfg.Limits.MinTrustedFollowers {
			return true, "rate limit exceeded"
		}
		return false, ""
	}

	relay.StoreEvent = func(ctx context.Context, event nostr2.Event) error {
		goEvent := toGoEvent(event)
		start := time.Now()
		if err := store.SaveEvent(ctx, goEvent); err != nil {
			if errors.Is(err, eventstore.ErrDupEvent) {
				return nostrEventstore.ErrDupEvent
			}
			return err
		}
		elapsed := time.Since(start)
		if elapsed > 100*time.Millisecond {
			pubkey := event.PubKey.Hex()
			if len(pubkey) > 8 {
				pubkey = pubkey[:8]
			}
			log.Printf("SLOW StoreEvent: kind=%d tags=%d elapsed=%v pubkey=%s", int(event.Kind), len(event.Tags), elapsed, pubkey)
		}
		statsTracker.RecordEventAccepted(int(event.Kind))
		return nil
	}

	relay.ReplaceEvent = func(ctx context.Context, event nostr2.Event) error {
		goEvent := toGoEvent(event)

		filter := nostr.Filter{
			Limit:   1,
			Kinds:   []int{goEvent.Kind},
			Authors: []string{goEvent.PubKey},
		}
		if nostr.IsAddressableKind(goEvent.Kind) {
			filter.Tags = nostr.TagMap{"d": []string{goEvent.Tags.GetD()}}
		}

		existing, err := store.QueryEvents(ctx, filter)
		if err != nil {
			return err
		}

		shouldStore := true
		for _, previous := range existing {
			if isOlderEvent(previous, goEvent) {
				if err := store.DeleteEvent(ctx, previous); err != nil {
					return err
				}
			} else {
				shouldStore = false
			}
		}

		if !shouldStore {
			return nil
		}

		start := time.Now()
		if err := store.SaveEvent(ctx, goEvent); err != nil {
			if errors.Is(err, eventstore.ErrDupEvent) {
				return nostrEventstore.ErrDupEvent
			}
			return err
		}
		elapsed := time.Since(start)
		if elapsed > 100*time.Millisecond {
			pubkey := event.PubKey.Hex()
			if len(pubkey) > 8 {
				pubkey = pubkey[:8]
			}
			log.Printf("SLOW ReplaceEvent: kind=%d tags=%d elapsed=%v pubkey=%s", int(event.Kind), len(event.Tags), elapsed, pubkey)
		}
		statsTracker.RecordEventAccepted(int(event.Kind))
		return nil
	}

	relay.QueryStored = func(ctx context.Context, filter nostr2.Filter) iter.Seq[nostr2.Event] {
		analyticsTracker.RecordREQ(toGoFilter(filter))

		// Track REQ kinds for stats and filter out disallowed kinds
		allowedKinds := make([]nostr2.Kind, 0, len(filter.Kinds))
		for _, kind := range filter.Kinds {
			kindInt := int(kind)
			statsTracker.RecordREQKind(ctx, kindInt)
			if !cfg.IsKindAllowed(kindInt) {
				statsTracker.RecordRejectedREQ(ctx, kindInt)
			} else {
				allowedKinds = append(allowedKinds, kind)
			}
		}

		// If no allowed kinds remain after filtering, return empty immediately
		if len(filter.Kinds) > 0 && len(allowedKinds) == 0 {
			return func(yield func(nostr2.Event) bool) {}
		}

		// Update filter with only allowed kinds
		filter.Kinds = allowedKinds

		goFilter := toGoFilter(filter)
		start := time.Now()
		events, err := store.QueryEvents(ctx, goFilter)
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			log.Printf("SLOW QueryEvents: kinds=%v authors=%d tags=%d limit=%d elapsed=%v results=%d",
				filter.Kinds, len(filter.Authors), len(filter.Tags), filter.Limit, elapsed, len(events))
		}
		if err != nil {
			log.Printf("QueryEvents failed: %v", err)
			return func(yield func(nostr2.Event) bool) {}
		}

		ip := khatru.GetIP(ctx)

		return func(yield func(nostr2.Event) bool) {
			var count int64
			for _, evt := range events {
				if ctx.Err() != nil {
					statsTracker.RecordEventsServed(context.Background(), ip, count)
					return
				}
				converted, err := toNostrEvent(evt)
				if err != nil {
					log.Printf("Failed to convert event %s: %v", evt.ID, err)
					continue
				}
				if !yield(converted) {
					statsTracker.RecordEventsServed(context.Background(), ip, count)
					return
				}
				count++
			}
			statsTracker.RecordEventsServed(context.Background(), ip, count)
		}
	}

	relay.DeleteEvent = func(ctx context.Context, id nostr2.ID) error {
		return store.DeleteEvent(ctx, &nostr.Event{ID: id.Hex()})
	}

	relay.Count = func(ctx context.Context, filter nostr2.Filter) (uint32, error) {
		count, err := store.CountEvents(ctx, toGoFilter(filter))
		if err != nil {
			return 0, err
		}
		if count <= 0 {
			return 0, nil
		}
		const maxUint32 = int64(^uint32(0))
		if count > maxUint32 {
			return ^uint32(0), nil
		}
		return uint32(count), nil
	}

	relay.CountHLL = func(ctx context.Context, filter nostr2.Filter, offset int) (uint32, *hyperloglog.HyperLogLog, error) {
		return store.CountEventsHLL(ctx, toGoFilter(filter), offset)
	}

	relay.OnConnect = func(ctx context.Context) {
		statsTracker.RecordConnection()
	}

	relay.OnDisconnect = func(ctx context.Context) {
		statsTracker.RecordDisconnection()
	}

	if cfg.Sync.Enabled && len(cfg.Sync.Relays) > 0 {
		syncKinds := cfg.Sync.Kinds
		if len(syncKinds) == 0 {
			syncKinds = cfg.SyncKinds
		}
		log.Printf("Starting initial sync from %d relays for %d kinds...", len(cfg.Sync.Relays), len(syncKinds))
		syncer := sync.NewSyncer(store, syncKinds, cfg.Sync.Relays)

		if testMode {
			log.Println("Test mode: running sync and exiting...")
			if err := syncer.SyncAll(context.Background()); err != nil {
				log.Printf("Sync error: %v", err)
				os.Exit(1)
			}
			log.Println("Sync completed successfully")
			os.Exit(0)
		}

		go func() {
			if err := syncer.SyncAll(context.Background()); err != nil {
				log.Printf("Sync error: %v", err)
			}
		}()
	} else if testMode {
		log.Println("Test mode enabled but sync disabled in config")
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	analyticsTracker.Start(ctx)

	log.Println("Relay: heavy analytics disabled in relay process - run './purplepages analytics' separately")

	// Record daily storage snapshots
	go func() {
		// Wait 5 minutes before first snapshot to ensure database is fully initialized
		log.Println("Storage tracking: waiting 5 minutes before first snapshot to ensure DB is initialized")
		time.Sleep(5 * time.Minute)

		if err := store.RecordDailyStorageSnapshot(ctx); err != nil {
			log.Printf("Failed to record initial storage snapshot: %v", err)
		} else {
			log.Println("Recorded initial storage snapshot")
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.RecordDailyStorageSnapshot(ctx); err != nil {
					log.Printf("Failed to record storage snapshot: %v", err)
				} else {
					log.Println("Recorded daily storage snapshot")
				}
			}
		}
	}()

	go func() {
		time.Sleep(2 * time.Minute)
		syncQueue.Start(ctx)
	}()

	// Persistent sync subscriber: keeps REQ open on all sync relays for all sync kinds
	var syncSubscriber *relay2.SyncSubscriber
	if cfg.Sync.Enabled && len(cfg.Sync.Relays) > 0 {
		syncSubKinds := cfg.Sync.Kinds
		if len(syncSubKinds) == 0 {
			syncSubKinds = cfg.SyncKinds
		}
		syncSubscriber = relay2.NewSyncSubscriber(store, cfg.Sync.Relays, syncSubKinds)
		go syncSubscriber.Start(ctx)
	}

	pageHandler := pages.NewHandler(store)

	analyticsHandler := stats.NewAnalyticsHandler(analyticsTracker, trustAnalyzer, store)
	dashboardHandler := stats.NewDashboardHandler(store)
	storageHandler := stats.NewStorageHandler(store)
	rejectionHandler := stats.NewRejectionHandler(store)
	communitiesHandler := stats.NewCommunitiesHandler(store)
	socialHandler := stats.NewSocialHandler(store)
	networkHandler := stats.NewNetworkHandler(store)
	timecapsuleHandler := pages.NewTimecapsuleHandler(store)

	// Password protection middleware for stats pages
	requireStatsAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if cfg.StatsPassword == "" {
				next(w, r)
				return
			}
			_, password, ok := r.BasicAuth()
			if !ok || password != cfg.StatsPassword {
				w.Header().Set("WWW-Authenticate", `Basic realm="Stats"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", relay.ServeHTTP)
	mux.HandleFunc("/rankings", pageHandler.HandleRankings)
	mux.HandleFunc("/profile", pageHandler.HandleProfile)
	mux.HandleFunc("/timecapsule", timecapsuleHandler.HandleTimecapsule())
	mux.HandleFunc("/stats", requireStatsAuth(statsTracker.HandleStats()))
	mux.HandleFunc("/stats/analytics", requireStatsAuth(analyticsHandler.HandleAnalytics()))
	mux.HandleFunc("/stats/analytics/purge", requireStatsAuth(analyticsHandler.HandlePurge()))
	mux.HandleFunc("/stats/dashboard", requireStatsAuth(dashboardHandler.HandleDashboard()))
	mux.HandleFunc("/stats/storage", requireStatsAuth(storageHandler.HandleStorage()))
	mux.HandleFunc("/stats/rejections", requireStatsAuth(rejectionHandler.HandleRejectionStats()))
	mux.HandleFunc("/stats/communities", requireStatsAuth(communitiesHandler.HandleCommunities()))
	mux.HandleFunc("/stats/social", requireStatsAuth(socialHandler.HandleSocial()))
	mux.HandleFunc("/stats/network", requireStatsAuth(networkHandler.HandleNetwork()))
	mux.HandleFunc("/relays", requireStatsAuth(statsTracker.HandleRelays()))
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "icon.png")
	})
	mux.HandleFunc("/icon.svg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "icon.svg")
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("Starting %s relay on %s", cfg.Relay.Name, server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down relay...")
	cancel()
	analyticsTracker.Stop()
	syncQueue.Stop()
	if syncSubscriber != nil {
		syncSubscriber.Stop()
	}

	if err := server.Shutdown(context.Background()); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}

func runAnalyticsWorker() {
	log.Println("Starting analytics worker process")

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	store, err := storage.New(cfg.Storage.Path, *cfg.Storage.ArchiveEnabled)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	if err := store.InitAnalyticsSchema(); err != nil {
		log.Fatalf("Failed to initialize analytics schema: %v", err)
	}
	if err := store.InitCacheSchema(); err != nil {
		log.Fatalf("Failed to initialize cache schema: %v", err)
	}
	if err := store.InitStorageStatsSchema(); err != nil {
		log.Fatalf("Failed to initialize storage stats schema: %v", err)
	}

	clusterDetector := analytics.NewClusterDetector(store)
	trustAnalyzer := analytics.NewTrustAnalyzer(store, clusterDetector, cfg.Limits.MinTrustedFollowers)
	communityDetector := analytics.NewCommunityDetector(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping analytics worker...")
		cancel()
	}()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run immediately if no trusted pubkeys, otherwise wait 5 minutes
	if trustAnalyzer.GetTrustedCount() == 0 {
		log.Println("No trusted pubkeys found, running trust analysis immediately")
		start := time.Now()
		followGraph := clusterDetector.BuildFollowGraph(ctx)
		log.Printf("follow graph build took %v", time.Since(start))
		start = time.Now()
		clusterDetector.DetectWithGraph(ctx, followGraph)
		log.Printf("clusterDetector.Detect took %v", time.Since(start))
		start = time.Now()
		trustAnalyzer.AnalyzeTrustWithGraph(ctx, followGraph)
		log.Printf("trustAnalyzer.AnalyzeTrust took %v", time.Since(start))
		start = time.Now()
		communityDetector.DetectCommunitiesWithGraph(ctx, followGraph)
		log.Printf("communityDetector.DetectCommunities took %v", time.Since(start))
		start = time.Now()
		if err := store.RefreshDerivedStats(ctx); err != nil {
			log.Printf("cache refresh failed: %v", err)
		} else {
			log.Printf("cache refresh took %v", time.Since(start))
		}
		if err := store.RecordDailyStorageSnapshot(ctx); err != nil {
			log.Printf("storage snapshot failed: %v", err)
		}
	} else {
		time.Sleep(5 * time.Minute)
	}

	log.Println("Analytics worker: starting hourly analysis loop")
	for {
		start := time.Now()
		followGraph := clusterDetector.BuildFollowGraph(ctx)
		log.Printf("follow graph build took %v", time.Since(start))
		start = time.Now()
		clusterDetector.DetectWithGraph(ctx, followGraph)
		log.Printf("clusterDetector.Detect took %v", time.Since(start))
		start = time.Now()
		trustAnalyzer.AnalyzeTrustWithGraph(ctx, followGraph)
		log.Printf("trustAnalyzer.AnalyzeTrust took %v", time.Since(start))
		start = time.Now()
		communityDetector.DetectCommunitiesWithGraph(ctx, followGraph)
		log.Printf("communityDetector.DetectCommunities took %v", time.Since(start))
		start = time.Now()
		if err := store.RefreshDerivedStats(ctx); err != nil {
			log.Printf("cache refresh failed: %v", err)
		} else {
			log.Printf("cache refresh took %v", time.Since(start))
		}
		if err := store.RecordDailyStorageSnapshot(ctx); err != nil {
			log.Printf("storage snapshot failed: %v", err)
		}

		select {
		case <-ctx.Done():
			log.Println("Analytics worker stopped")
			return
		case <-ticker.C:
		}
	}
}

func runSyncCommand(args []string) {
	syncFlags := flag.NewFlagSet("sync", flag.ExitOnError)
	kinds := syncFlags.String("k", "", "Comma-separated list of kinds to sync (e.g., -k 0,3,10002)")
	direction := syncFlags.String("d", "down", "Sync direction: down (pull from relay), up (push to relay), both")
	syncFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: purplepages sync [options] <relay-url>\n\n")
		fmt.Fprintf(os.Stderr, "Trigger a negentropy sync with a relay.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		syncFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  purplepages sync -k 3 relay.example.com\n")
		fmt.Fprintf(os.Stderr, "  purplepages sync -k 0,3,10002 wss://relay.example.com\n")
		fmt.Fprintf(os.Stderr, "  purplepages sync -k 10002 -d both relay.example.com\n")
	}

	if err := syncFlags.Parse(args); err != nil {
		os.Exit(1)
	}

	if syncFlags.NArg() < 1 {
		syncFlags.Usage()
		os.Exit(1)
	}

	relayURL := syncFlags.Arg(0)
	if !strings.HasPrefix(relayURL, "ws://") && !strings.HasPrefix(relayURL, "wss://") {
		relayURL = "wss://" + relayURL
	}

	// Parse kinds
	var kindsToSync []int
	if *kinds != "" {
		for _, k := range strings.Split(*kinds, ",") {
			k = strings.TrimSpace(k)
			var kind int
			if _, err := fmt.Sscanf(k, "%d", &kind); err != nil {
				log.Fatalf("Invalid kind: %s", k)
			}
			kindsToSync = append(kindsToSync, kind)
		}
	}

	// Parse direction
	var dir nip77.Direction
	switch *direction {
	case "down":
		dir = nip77.Down
	case "up":
		dir = nip77.Up
	case "both":
		dir = nip77.Both
	default:
		log.Fatalf("Invalid direction: %s (use: down, up, both)", *direction)
	}

	// Load config
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// If no kinds specified, use sync kinds from config
	if len(kindsToSync) == 0 {
		kindsToSync = cfg.SyncKinds
		log.Printf("No kinds specified, using sync kinds: %v", kindsToSync)
	}

	// Open storage
	store, err := storage.New(cfg.Storage.Path, *cfg.Storage.ArchiveEnabled)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Get the underlying eventstore for nip77
	wrapper := eventstore.RelayWrapper{Store: store.EventStore()}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dirLabel := map[nip77.Direction]string{nip77.Down: "pulling from", nip77.Up: "pushing to", nip77.Both: "syncing with"}[dir]

	for _, kind := range kindsToSync {
		filter := nostr.Filter{Kinds: []int{kind}}
		log.Printf("Negentropy sync: %s %s for kind %d...", dirLabel, relayURL, kind)

		if err := nip77.NegentropySync(ctx, wrapper, relayURL, filter, dir); err != nil {
			log.Printf("Failed to sync kind %d: %v", kind, err)
			continue
		}
		log.Printf("Kind %d sync complete", kind)
	}

	log.Println("Sync complete")
}

func importEventsFromJSONL(store *storage.Storage, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	ctx := context.Background()
	count := 0
	skipped := 0
	failed := 0
	rescued := 0
	lineNum := 0
	needsUnescape := false

	log.Printf("Starting import from %s...", filePath)

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event nostr.Event
		if needsUnescape {
			line = bytes.ReplaceAll(line, []byte("\\\\"), []byte("\\"))
		}
		if err := json.Unmarshal(line, &event); err != nil {
			if !needsUnescape {
				// Legacy exports may double-escape content; try a best-effort unescape.
				fixed := bytes.ReplaceAll(line, []byte("\\\\"), []byte("\\"))
				if err = json.Unmarshal(fixed, &event); err != nil {
					failed++
					if failed <= 5 || failed%10000 == 0 {
						log.Printf("Failed to parse event on line %d: %v", lineNum, err)
					}
					continue
				}
				needsUnescape = true
				rescued++
			} else {
				failed++
				if failed <= 5 || failed%10000 == 0 {
					log.Printf("Failed to parse event on line %d: %v", lineNum, err)
				}
				continue
			}
		} else if needsUnescape {
			rescued++
		}

		if err := store.SaveEvent(ctx, &event); err != nil {
			if err.Error() == "duplicate: event already exists" {
				skipped++
			} else if strings.Contains(err.Error(), "MDB_MAP_FULL") {
				return fmt.Errorf("lmdb map full after %d imports (line %d): %w", count, lineNum, err)
			} else {
				log.Printf("Failed to save event %s: %v", event.ID, err)
				failed++
			}
			continue
		}

		count++
		if count%1000 == 0 {
			log.Printf("Imported %d events (%d skipped, %d failed, %d rescued)...", count, skipped, failed, rescued)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	log.Printf("Import complete: %d events imported, %d skipped (duplicates), %d failed, %d rescued", count, skipped, failed, rescued)
	return nil
}

func toGoEvent(event nostr2.Event) *nostr.Event {
	return &nostr.Event{
		ID:        event.ID.Hex(),
		PubKey:    event.PubKey.Hex(),
		CreatedAt: nostr.Timestamp(event.CreatedAt),
		Kind:      int(event.Kind),
		Tags:      toGoTags(event.Tags),
		Content:   event.Content,
		Sig:       nostr2.HexEncodeToString(event.Sig[:]),
	}
}

func toGoTags(tags nostr2.Tags) nostr.Tags {
	if len(tags) == 0 {
		return nil
	}
	out := make(nostr.Tags, len(tags))
	for i, tag := range tags {
		out[i] = nostr.Tag(tag)
	}
	return out
}

func toGoFilter(filter nostr2.Filter) nostr.Filter {
	goFilter := nostr.Filter{
		Limit:     filter.Limit,
		Search:    filter.Search,
		LimitZero: filter.LimitZero,
	}
	if len(filter.IDs) > 0 {
		goFilter.IDs = make([]string, len(filter.IDs))
		for i, id := range filter.IDs {
			goFilter.IDs[i] = id.Hex()
		}
	}
	if len(filter.Kinds) > 0 {
		goFilter.Kinds = make([]int, len(filter.Kinds))
		for i, kind := range filter.Kinds {
			goFilter.Kinds[i] = int(kind)
		}
	}
	if len(filter.Authors) > 0 {
		goFilter.Authors = make([]string, len(filter.Authors))
		for i, author := range filter.Authors {
			goFilter.Authors[i] = author.Hex()
		}
	}
	if len(filter.Tags) > 0 {
		goFilter.Tags = make(nostr.TagMap, len(filter.Tags))
		for key, values := range filter.Tags {
			goFilter.Tags[key] = append([]string(nil), values...)
		}
	}
	if filter.Since != 0 {
		since := nostr.Timestamp(filter.Since)
		goFilter.Since = &since
	}
	if filter.Until != 0 {
		until := nostr.Timestamp(filter.Until)
		goFilter.Until = &until
	}
	return goFilter
}

func toNostrEvent(event *nostr.Event) (nostr2.Event, error) {
	if event == nil {
		return nostr2.Event{}, fmt.Errorf("nil event")
	}
	id, err := nostr2.IDFromHex(event.ID)
	if err != nil {
		return nostr2.Event{}, fmt.Errorf("invalid event id %q: %w", event.ID, err)
	}
	pubkey, err := nostr2.PubKeyFromHex(event.PubKey)
	if err != nil {
		return nostr2.Event{}, fmt.Errorf("invalid pubkey %q: %w", event.PubKey, err)
	}
	var sig [64]byte
	if event.Sig != "" {
		sigBytes, err := nostr2.HexDecodeString(event.Sig)
		if err != nil {
			return nostr2.Event{}, fmt.Errorf("invalid signature: %w", err)
		}
		if len(sigBytes) != len(sig) {
			return nostr2.Event{}, fmt.Errorf("invalid signature length: %d", len(sigBytes))
		}
		copy(sig[:], sigBytes)
	}

	return nostr2.Event{
		ID:        id,
		PubKey:    pubkey,
		CreatedAt: nostr2.Timestamp(event.CreatedAt),
		Kind:      nostr2.Kind(event.Kind),
		Tags:      toNostrTags(event.Tags),
		Content:   event.Content,
		Sig:       sig,
	}, nil
}

func toNostrTags(tags nostr.Tags) nostr2.Tags {
	if len(tags) == 0 {
		return nil
	}
	out := make(nostr2.Tags, len(tags))
	for i, tag := range tags {
		out[i] = nostr2.Tag(tag)
	}
	return out
}

func isOlderEvent(previous, next *nostr.Event) bool {
	return previous.CreatedAt < next.CreatedAt ||
		(previous.CreatedAt == next.CreatedAt && previous.ID > next.ID)
}
