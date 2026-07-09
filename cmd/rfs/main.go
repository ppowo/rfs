package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ppowo/rfs/internal/rfs"
	"github.com/ppowo/rfs/internal/sources"
)

func shouldEnableSelfUpdate(buildVersion string, flagEnabled bool) bool {
	return flagEnabled && buildVersion != "dev"
}

func pollCycleDetails(selfUpdateEnabled bool, sources []rfs.Source) string {
	sourceDetail := fmt.Sprintf("%d source polls", len(sources))
	if len(sources) == 1 {
		sourceDetail = fmt.Sprintf("source poll %s", sources[0].ID)
	}
	if selfUpdateEnabled {
		return "self-update check + " + sourceDetail
	}
	return sourceDetail
}

func logNextPollCycle(started time.Time, interval time.Duration, selfUpdateEnabled bool, sources []rfs.Source) {
	next := started.Add(interval)
	remaining := time.Until(next).Round(time.Second)
	if remaining < 0 {
		remaining = 0
	}
	log.Printf("next poll cycle in %s at %s (%s)", remaining, next.Format("15:04:05"), pollCycleDetails(selfUpdateEnabled, sources))
}

func main() {
	addr := flag.String("addr", ":14298", "HTTP listen address")
	interval := flag.Duration("interval", time.Hour, "global source polling interval")
	domainSpacing := flag.Duration("domain-spacing", 10*time.Second, "minimum spacing between requests to the same domain")
	dbPath := flag.String("db", "", "SQLite database path; defaults to the OS user cache dir, or use :memory:")
	showVersion := flag.Bool("version", false, "print build version and exit")
	selfUpdate := flag.Bool("self-update", true, "enable self-update checks for non-dev builds")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildInfo.String())
		return
	}
	if *interval <= 0 {
		log.Fatalf("interval must be positive, got %s", *interval)
	}
	if *domainSpacing < 0 {
		log.Fatalf("domain-spacing must not be negative, got %s", *domainSpacing)
	}

	log.Print(buildInfo.String())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := openStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	registeredSources := sources.All()
	poller := rfs.Poller{
		Fetcher: rfs.NewHTTPFetcher(nil),
		Store:   store,
	}
	gate := rfs.NewDomainGate(nil, rfs.DomainGateConfig{
		MinSpacing:     *domainSpacing,
		InitialBackoff: time.Minute,
		MaxBackoff:     time.Hour,
	})

	// Self-update (ADR 0004): for a deployed build (version baked in by the
	// release pipeline), check the latest GitHub release at the start of each
	// poll cycle. A "dev" local build always skips self-update — there is no
	// point swapping a `go run` temp binary, and it lets local development poll
	// without hitting the GitHub API. Use -self-update=false to also disable it
	// for a release-like local build.
	const selfUpdateRepo = "ppowo/rfs"
	var updater *rfs.Updater
	var reexecPath string
	if shouldEnableSelfUpdate(version, *selfUpdate) {
		swapper, err := rfs.NewFileSwapper()
		if err != nil {
			log.Printf("self-update disabled: %v", err)
		} else {
			// Capture the install path before any swap. After Swap renames the
			// running inode to .bak, a fresh os.Executable() can point at the
			// backup, so re-exec must use this original install path.
			reexecPath = swapper.Path()
			updater = &rfs.Updater{
				Current:    version,
				GOOS:       runtime.GOOS,
				GOARCH:     runtime.GOARCH,
				Source:     rfs.NewGitHubReleaseSource(selfUpdateRepo),
				Downloader: rfs.NewHTTPSDownloader(),
				Swapper:    swapper,
			}
			log.Printf("self-update enabled: current version %s, checking %s releases", version, selfUpdateRepo)
		}
	} else if version == "dev" {
		log.Printf("self-update disabled: dev build")
	} else {
		log.Printf("self-update disabled: -self-update=false")
	}

	log.Printf("serving feeds on %s", *addr)
	logFeeds(*addr, registeredSources)
	log.Printf("poll schedule: first cycle starts immediately, then every %s (%s)", *interval, pollCycleDetails(updater != nil, registeredSources))

	// reexec is closed when a self-update is applied, signalling main to drain
	// (via stop(), the same path a SIGTERM takes) and then syscall.Exec the
	// freshly-swapped binary in place. sync.Once guards against double-close if
	// multiple polls apply an update before the watcher acts on it.
	reexec := make(chan struct{})
	var reexecOnce sync.Once
	requestReexec := func() { reexecOnce.Do(func() { close(reexec) }) }

	loop := rfs.Loop{
		Poll: func(ctx context.Context) {
			started := time.Now()
			if updater != nil {
				result, err := updater.Check(ctx)
				if err != nil {
					log.Printf("self-update: %v", err)
				} else if result.Latest == "" {
					log.Printf("self-update: no releases found; current %s", result.Current)
				} else if result.Applied {
					log.Printf("self-update: installed release %s over current %s; draining and re-executing", result.Latest, result.Current)
					requestReexec()
					return // re-exec is imminent; skip polling this cycle
				} else {
					log.Printf("self-update: latest release %s, current %s; no update", result.Latest, result.Current)
				}
			}
			pollAll(ctx, registeredSources, poller, gate)
			logNextPollCycle(started, *interval, updater != nil, registeredSources)
		},
		Interval: *interval,
	}
	loopDone := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(loopDone)
	}()

	// A self-update applies by triggering the same graceful shutdown a signal
	// does: stop() cancels ctx, the loop drains its in-flight poll (B), the
	// server shuts down, then main re-execs. Without reexec this goroutine is
	// inert; a SIGTERM cancels ctx through signal.NotifyContext instead.
	go func() {
		select {
		case <-reexec:
			stop()
		case <-ctx.Done():
		}
	}()

	server := &http.Server{
		Addr:    *addr,
		Handler: rfs.NewHTTPHandler(store, registeredSources, buildInfo),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown server: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}

	<-loopDone // wait for the poll loop to drain before the process exits

	// If a self-update was applied, the new binary is on disk at reexecPath
	// (captured before the swap); replace this process image with it in place
	// (same PID, no restart).
	select {
	case <-reexec:
		if reexecPath == "" {
			log.Fatalf("self-update: missing re-exec path")
		}
		if err := syscall.Exec(reexecPath, os.Args, os.Environ()); err != nil {
			log.Fatalf("self-update: re-exec %s: %v", reexecPath, err)
		}
	default:
	}
}

func pollAll(ctx context.Context, sources []rfs.Source, poller rfs.Poller, gate *rfs.DomainGate) {
	var wg sync.WaitGroup
	for _, source := range sources {
		source := source
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := pollOne(ctx, source, poller, gate); err != nil {
				log.Printf("poll %s: %v", source.ID, err)
			}
		}()
	}
	wg.Wait()
}

func pollOne(ctx context.Context, source rfs.Source, poller rfs.Poller, gate *rfs.DomainGate) error {
	domain, err := sourceDomain(source)
	if err != nil {
		return err
	}

	result, err := gate.Run(ctx, domain, func() (rfs.PollResult, error) {
		return poller.Poll(ctx, source)
	})
	if err != nil {
		return err
	}

	switch result.Status {
	case rfs.PollUpdated:
		log.Printf("poll %s: updated", source.ID)
	case rfs.PollUnchanged:
		log.Printf("poll %s: unchanged", source.ID)
	case rfs.PollThrottled:
		log.Printf("poll %s: throttled, retry after %s", source.ID, result.RetryAfter)
	}
	return nil
}

func sourceDomain(source rfs.Source) (string, error) {
	parsed, err := url.Parse(source.URL)
	if err != nil {
		return "", err
	}
	return parsed.Hostname(), nil
}

func logFeeds(addr string, sources []rfs.Source) {
	baseURL := feedBaseURL(addr)
	log.Printf("index:  %s/", baseURL)
	log.Printf("feeds:")
	for _, source := range sources {
		log.Printf("  %s\n    %s/feeds/%s.xml\n    %s/feeds/%s.html",
			source.ID, baseURL, source.ID, baseURL, source.ID)
	}
}

func feedBaseURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	return "http://" + addr
}

func openStore(path string) (*rfs.SQLiteStore, error) {
	if path == "" {
		resolved, err := defaultDBPath()
		if err != nil {
			return nil, err
		}
		path = resolved
	}

	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	return rfs.OpenSQLiteStore(path)
}

func defaultDBPath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(cacheDir, "rfs", "rfs.sqlite"), nil
}
