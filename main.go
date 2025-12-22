package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fiatjaf/eventstore"
	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip11"
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

	port := flag.Int("port", 0, "Override port from config (use 9999 for sync-only test mode)")
	importFile := flag.String("import", "", "Import events from JSONL file and exit")
	testHydrator := flag.Bool("test-hydrator", false, "Run profile hydrator once and show results")
	benchmarkHydrator := flag.Bool("benchmark-hydrator", false, "Benchmark hydrator performance on production DB")
	flag.Parse()

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Handle test-hydrator mode early (doesn't need production DB)
	if *testHydrator {
		if err := runHydratorTestWithCopy(cfg); err != nil {
			log.Fatalf("Hydrator test failed: %v", err)
		}
		os.Exit(0)
	}

	// Handle benchmark mode
	if *benchmarkHydrator {
		if err := runHydratorBenchmark(cfg); err != nil {
			log.Fatalf("Benchmark failed: %v", err)
		}
		os.Exit(0)
	}

	if *port != 0 {
		cfg.Server.Port = *port
	}

	testMode := cfg.Server.Port == 9999

	store, err := storage.New(cfg.Storage.Backend, cfg.Storage.Path)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	if err := store.InitRelayDiscoverySchema(); err != nil {
		log.Fatalf("Failed to initialize relay discovery schema: %v", err)
	}

	if err := store.InitProfileHydrationSchema(); err != nil {
		log.Fatalf("Failed to initialize profile hydration schema: %v", err)
	}

	if err := store.InitAnalyticsSchema(); err != nil {
		log.Fatalf("Failed to initialize analytics schema: %v", err)
	}

	if err := store.InitTrustedSyncSchema(); err != nil {
		log.Fatalf("Failed to initialize trusted sync schema: %v", err)
	}

	if *importFile != "" {
		if err := importEventsFromJSONL(store, *importFile); err != nil {
			log.Fatalf("Failed to import events: %v", err)
		}
		log.Println("Import completed successfully")
		os.Exit(0)
	}

	statsTracker := stats.New(store)
	analyticsTracker := analytics.NewTracker(store)
	clusterDetector := analytics.NewClusterDetector(store)
	trustAnalyzer := analytics.NewTrustAnalyzer(store, clusterDetector, 10)
	discovery := relay2.NewDiscovery(store)
	syncQueue := relay2.NewSyncQueue(store, cfg.SyncKinds)

	relay := khatru.NewRelay()

	relay.Info.Name = cfg.Relay.Name
	relay.Info.Description = cfg.Relay.Description
	relay.Info.PubKey = cfg.Relay.Pubkey
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

	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		if !cfg.IsKindAllowed(event.Kind) {
			statsTracker.RecordEventRejected()
			return true, fmt.Sprintf("kind %d is not allowed", event.Kind)
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
	})

	relay.RejectFilter = append(relay.RejectFilter, func(ctx context.Context, filter nostr.Filter) (bool, string) {
		if filter.Limit > cfg.Limits.MaxLimit {
			return true, fmt.Sprintf("limit too high: %d (max %d)", filter.Limit, cfg.Limits.MaxLimit)
		}
		return false, ""
	})

	relay.StoreEvent = append(relay.StoreEvent, func(ctx context.Context, event *nostr.Event) error {
		if err := store.SaveEvent(ctx, event); err != nil {
			return err
		}
		statsTracker.RecordEventAccepted(event.Kind)
		return nil
	})

	relay.OnEventSaved = append(relay.OnEventSaved, func(ctx context.Context, event *nostr.Event) {
		if event.Kind == 10002 {
			discovery.ExtractRelaysFromEvent(ctx, event)
		}
	})

	relay.QueryEvents = append(relay.QueryEvents, func(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
		analyticsTracker.RecordREQ(filter)

		events, err := store.QueryEvents(ctx, filter)
		if err != nil {
			return nil, err
		}

		ch := make(chan *nostr.Event)
		go func() {
			defer close(ch)
			for _, evt := range events {
				select {
				case ch <- evt:
				case <-ctx.Done():
					return
				}
			}
		}()

		return ch, nil
	})

	relay.DeleteEvent = append(relay.DeleteEvent, func(ctx context.Context, event *nostr.Event) error {
		return store.DeleteEvent(ctx, event)
	})

	relay.OnConnect = append(relay.OnConnect, func(ctx context.Context) {
		statsTracker.RecordConnection()
	})

	relay.OnDisconnect = append(relay.OnDisconnect, func(ctx context.Context) {
		statsTracker.RecordDisconnection()
	})

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
	}

	if testMode {
		log.Println("Test mode enabled but sync disabled in config")
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	analyticsTracker.Start(ctx)

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		time.Sleep(5 * time.Minute)
		for {
			clusterDetector.Detect(ctx)
			trustAnalyzer.AnalyzeTrust(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	go func() {
		time.Sleep(2 * time.Minute)
		syncQueue.Start(ctx)
	}()

	var hydrator *relay2.ProfileHydrator
	if cfg.ProfileHydration.Enabled && len(cfg.Sync.Relays) > 0 {
		hydrator = relay2.NewProfileHydrator(
			store,
			cfg.Sync.Relays,
			cfg.ProfileHydration.MinFollowers,
			cfg.ProfileHydration.RetryAfterHours,
			cfg.ProfileHydration.BatchSize,
		)
		go func() {
			time.Sleep(3 * time.Minute) // Wait a bit after startup
			hydrator.Start(ctx, cfg.ProfileHydration.IntervalMinutes)
		}()
	}

	var trustedSyncer *relay2.TrustedSyncer
	if !cfg.TrustedSync.Disabled {
		trustedSyncer = relay2.NewTrustedSyncer(
			store,
			trustAnalyzer,
			cfg.TrustedSync.Kinds,
			cfg.TrustedSync.BatchSize,
			cfg.TrustedSync.TimeoutSeconds,
		)
		go func() {
			time.Sleep(6 * time.Minute) // Wait for trust analyzer to run first
			trustedSyncer.Start(ctx, cfg.TrustedSync.IntervalMinutes)
		}()
	}

	pageHandler := pages.NewHandler(store)

	analyticsHandler := stats.NewAnalyticsHandler(analyticsTracker, trustAnalyzer, store)
	trustedSyncHandler := stats.NewTrustedSyncHandler(store)

	mux := http.NewServeMux()
	mux.HandleFunc("/", relay.ServeHTTP)
	mux.HandleFunc("/rankings", pageHandler.HandleRankings)
	mux.HandleFunc("/search", pageHandler.HandleSearch)
	mux.HandleFunc("/profile", pageHandler.HandleProfile)
	mux.HandleFunc("/stats", statsTracker.HandleStats(cfg.AllowedKinds.ToSlice()))
	mux.HandleFunc("/stats/analytics", analyticsHandler.HandleAnalytics())
	mux.HandleFunc("/stats/analytics/purge", analyticsHandler.HandlePurge())
	mux.HandleFunc("/stats/trusted-sync", trustedSyncHandler.HandleTrustedSyncStats())
	mux.HandleFunc("/relays", statsTracker.HandleRelays())
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
	if hydrator != nil {
		hydrator.Stop()
	}
	if trustedSyncer != nil {
		trustedSyncer.Stop()
	}

	if err := server.Shutdown(context.Background()); err != nil {
		log.Printf("Server shutdown error: %v", err)
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
	store, err := storage.New(cfg.Storage.Backend, cfg.Storage.Path)
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

	log.Printf("Starting import from %s...", filePath)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event nostr.Event
		if err := json.Unmarshal(line, &event); err != nil {
			log.Printf("Failed to parse event on line %d: %v", count+1, err)
			failed++
			continue
		}

		if err := store.SaveEvent(ctx, &event); err != nil {
			if err.Error() == "duplicate: event already exists" {
				skipped++
			} else {
				log.Printf("Failed to save event %s: %v", event.ID, err)
				failed++
			}
			continue
		}

		count++
		if count%1000 == 0 {
			log.Printf("Imported %d events (%d skipped, %d failed)...", count, skipped, failed)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	log.Printf("Import complete: %d events imported, %d skipped (duplicates), %d failed", count, skipped, failed)
	return nil
}

func runHydratorTestWithCopy(cfg *config.Config) error {
	testDBPath := "./data/purplepages_test.db"

	log.Println("Creating fresh test database with sample follower graph...")

	// Remove old test database if it exists
	os.Remove(testDBPath)

	// Create a fresh test database
	testStore, err := storage.New(cfg.Storage.Backend, testDBPath)
	if err != nil {
		return fmt.Errorf("failed to create test database: %w", err)
	}
	defer testStore.Close()
	defer os.Remove(testDBPath) // Clean up after test

	// Initialize schemas
	if err := testStore.InitRelayDiscoverySchema(); err != nil {
		return fmt.Errorf("failed to initialize relay discovery schema: %w", err)
	}
	if err := testStore.InitProfileHydrationSchema(); err != nil {
		return fmt.Errorf("failed to initialize profile hydration schema: %w", err)
	}

	ctx := context.Background()

	// Set min_followers to 3 for this test
	originalMinFollowers := cfg.ProfileHydration.MinFollowers
	cfg.ProfileHydration.MinFollowers = 3
	defer func() { cfg.ProfileHydration.MinFollowers = originalMinFollowers }()

	// Use REAL pubkey: npub1l2vyh47mk2p0qlsku7hg0vn29faehy9hy34ygaclpn66ukqp3afqutajft
	target := "fa984bd7dbb282f07e16e7ae87b26a2a7b9b90b7246a44771f0cf5ae58018f52"

	// Create some fake followers for the target
	follower1 := "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"
	follower2 := "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"
	follower3 := "cccc3333cccc3333cccc3333cccc3333cccc3333cccc3333cccc3333cccc3333"

	log.Printf("Test scenario:")
	log.Printf("  Target: %s... (REAL npub - has data on relays!)", target[:16])
	log.Printf("  Creating 3 kind:3 events where followers follow this real target")
	log.Printf("  Will actually FETCH missing data from %v", cfg.Sync.Relays)
	log.Println()

	// Create kind 3 events - each follower follows the target
	now := nostr.Now()
	for i, follower := range []string{follower1, follower2, follower3} {
		evt := &nostr.Event{
			PubKey:    follower,
			CreatedAt: now - nostr.Timestamp(i),
			Kind:      3,
			Tags: nostr.Tags{
				nostr.Tag{"p", target},
			},
			Content: "",
		}
		evt.ID = evt.GetID()

		if err := testStore.SaveEvent(ctx, evt); err != nil {
			log.Printf("Warning: failed to save test event: %v", err)
		}
	}

	log.Println("Test data created")
	log.Println()

	// Override sync relays to use public relays with real data
	originalRelays := cfg.Sync.Relays
	cfg.Sync.Relays = []string{"wss://relay.damus.io", "wss://nos.lol"}
	defer func() { cfg.Sync.Relays = originalRelays }()

	log.Printf("Using public relays for fetch: %v", cfg.Sync.Relays)
	log.Println()

	// Run the test and ACTUALLY FETCH from the relay!
	return runHydratorTest(testStore, cfg, false)
}

func runHydratorBenchmark(cfg *config.Config) error {
	log.Println("=== Hydrator Performance Benchmark ===")
	log.Println()

	// Open production database (read-only operations)
	store, err := storage.New(cfg.Storage.Backend, cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Get database stats
	kind0Count, _ := store.CountEvents(ctx, 0)
	kind3Count, _ := store.CountEvents(ctx, 3)
	kind10002Count, _ := store.CountEvents(ctx, 10002)

	log.Printf("Database size:")
	log.Printf("  Kind 0 (profiles): %d", kind0Count)
	log.Printf("  Kind 3 (contacts): %d", kind3Count)
	log.Printf("  Kind 10002 (relays): %d", kind10002Count)
	log.Println()

	// Benchmark follower counting query
	log.Println("Benchmarking follower counting query...")
	log.Printf("  Min followers: %d", cfg.ProfileHydration.MinFollowers)

	start := time.Now()
	followerCounts, err := store.GetFollowerCounts(ctx, cfg.ProfileHydration.MinFollowers)
	duration := time.Since(start)

	if err != nil {
		return fmt.Errorf("follower counting failed: %w", err)
	}

	log.Printf("  ✓ Completed in %v", duration)
	log.Printf("  Found %d pubkeys with %d+ followers", len(followerCounts), cfg.ProfileHydration.MinFollowers)
	log.Println()

	// Benchmark full analysis (what the hydrator does)
	log.Println("Benchmarking full hydrator analysis...")

	hydrator := relay2.NewProfileHydrator(
		store,
		cfg.Sync.Relays,
		cfg.ProfileHydration.MinFollowers,
		cfg.ProfileHydration.RetryAfterHours,
		cfg.ProfileHydration.BatchSize,
	)

	start = time.Now()
	pubkeysToFetch := hydrator.FindPubkeysNeedingHydration(ctx)
	duration = time.Since(start)

	log.Printf("  ✓ Completed in %v", duration)
	log.Printf("  Found %d pubkeys needing hydration", len(pubkeysToFetch))
	log.Println()

	log.Println("=== Benchmark Complete ===")
	return nil
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

func runHydratorTest(store *storage.Storage, cfg *config.Config, skipFetch bool) error {
	ctx := context.Background()

	log.Println("=== Profile Hydrator Test ===")
	log.Println()

	// Show configuration
	log.Printf("Configuration:")
	log.Printf("  Min Followers: %d", cfg.ProfileHydration.MinFollowers)
	log.Printf("  Retry After: %d hours", cfg.ProfileHydration.RetryAfterHours)
	log.Printf("  Batch Size: %d", cfg.ProfileHydration.BatchSize)
	log.Printf("  Relays: %v", cfg.Sync.Relays)
	log.Printf("  Skip Fetch: %t (only show what would be fetched)", skipFetch)
	log.Println()

	// Show before stats
	beforeAttempts, err := store.GetProfileFetchAttemptCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to count attempts: %w", err)
	}

	totalKind0, _ := store.CountEvents(ctx, 0)
	totalKind3, _ := store.CountEvents(ctx, 3)
	totalKind10002, _ := store.CountEvents(ctx, 10002)

	log.Printf("Before hydration:")
	log.Printf("  Previous fetch attempts: %d", beforeAttempts)
	log.Printf("  Current events - kind 0: %d, kind 3: %d, kind 10002: %d", totalKind0, totalKind3, totalKind10002)
	log.Println()

	// Create and run hydrator
	if len(cfg.Sync.Relays) == 0 {
		return fmt.Errorf("no sync relays configured")
	}

	hydrator := relay2.NewProfileHydrator(
		store,
		cfg.Sync.Relays,
		cfg.ProfileHydration.MinFollowers,
		cfg.ProfileHydration.RetryAfterHours,
		cfg.ProfileHydration.BatchSize,
	)

	// First, show what would be fetched
	log.Println("Analyzing which pubkeys need hydration...")
	analysisStart := time.Now()
	pubkeysToFetch := hydrator.FindPubkeysNeedingHydration(ctx)
	analysisDuration := time.Since(analysisStart)
	log.Printf("Analysis completed in %v", analysisDuration)

	if len(pubkeysToFetch) == 0 {
		log.Println("No pubkeys need hydration at this time.")
		log.Println()
		log.Println("Reasons why pubkeys might not need hydration:")
		log.Println("  - All pubkeys with 10+ followers already have all required event kinds")
		log.Println("  - Recent fetch attempts (within 24 hours) already cover all eligible pubkeys")
		log.Println()
		log.Println("Current status is good - the hydrator has done its job!")
	} else {
		log.Printf("Found %d pubkeys that need hydration:", len(pubkeysToFetch))
		limit := 10
		if len(pubkeysToFetch) < limit {
			limit = len(pubkeysToFetch)
		}
		for i := 0; i < limit; i++ {
			need := pubkeysToFetch[i]
			log.Printf("  %s - need k0:%t k3:%t k10002:%t",
				need.Pubkey[:16], need.NeedKind0, need.NeedKind3, need.NeedKind10002)
		}
		if len(pubkeysToFetch) > limit {
			log.Printf("  ...and %d more", len(pubkeysToFetch)-limit)
		}
		log.Println()

		if !skipFetch {
			log.Println("Running profile hydrator to fetch missing data...")
			log.Println()
			hydrator.RunOnce(ctx)
			log.Println()
		} else {
			log.Println("Skipping actual fetch (dry-run mode)")
			log.Println()
		}
	}

	// Show after stats
	afterAttempts, err := store.GetProfileFetchAttemptCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to count attempts: %w", err)
	}

	newKind0, _ := store.CountEvents(ctx, 0)
	newKind3, _ := store.CountEvents(ctx, 3)
	newKind10002, _ := store.CountEvents(ctx, 10002)

	if !skipFetch && len(pubkeysToFetch) > 0 {
		log.Printf("After hydration:")
		log.Printf("  Total fetch attempts: %d (new: %d)", afterAttempts, afterAttempts-beforeAttempts)
		log.Printf("  Current events - kind 0: %d (+%d), kind 3: %d (+%d), kind 10002: %d (+%d)",
			newKind0, newKind0-totalKind0,
			newKind3, newKind3-totalKind3,
			newKind10002, newKind10002-totalKind10002)
		log.Println()
	}

	// Show some example attempts
	log.Println("Recent fetch attempts:")
	attempts, err := store.GetRecentProfileFetchAttempts(ctx, 10)
	if err != nil {
		return fmt.Errorf("failed to query attempts: %w", err)
	}

	for _, attempt := range attempts {
		timestamp := time.Unix(attempt.LastAttempt, 0).Format("2006-01-02 15:04:05")
		log.Printf("  %s... @ %s - k0:%t k3:%t k10002:%t",
			attempt.Pubkey[:16], timestamp, attempt.FetchedKind0, attempt.FetchedKind3, attempt.FetchedKind10002)
	}

	log.Println()
	log.Println("=== Test Complete ===")

	return nil
}
