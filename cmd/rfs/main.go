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

func pollCycleDetails(sources []rfs.Source) string {
	sourceDetail := fmt.Sprintf("%d source polls", len(sources))
	if len(sources) == 1 {
		sourceDetail = fmt.Sprintf("source poll %s", sources[0].ID)
	}
	return sourceDetail
}

func logNextPollCycle(started time.Time, interval time.Duration, sources []rfs.Source) {
	next := started.Add(interval)
	remaining := time.Until(next).Round(time.Second)
	if remaining < 0 {
		remaining = 0
	}
	log.Printf("next poll cycle in %s at %s (%s)", remaining, next.Format("15:04:05"), pollCycleDetails(sources))
}

func main() {
	addr := flag.String("addr", ":14298", "HTTP listen address")
	interval := flag.Duration("interval", time.Hour, "global source polling interval")
	domainSpacing := flag.Duration("domain-spacing", 10*time.Second, "minimum spacing between requests to the same domain")
	dbPath := flag.String("db", "", "SQLite database path; defaults to the OS user cache dir, or use :memory:")
	showVersion := flag.Bool("version", false, "print build version and exit")
	selfUpdate := flag.Bool("self-update", true, "enable self-update checks for non-dev builds")
	selfUpdateInterval := flag.Duration("self-update-interval", time.Hour, "how often to check for a self-update")
	selfUpdateTimeout := flag.Duration("self-update-timeout", 30*time.Second, "maximum duration of one self-update check")
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
	if *selfUpdateInterval <= 0 {
		log.Fatalf("self-update-interval must be positive, got %s", *selfUpdateInterval)
	}
	if *selfUpdateTimeout <= 0 {
		log.Fatalf("self-update-timeout must be positive, got %s", *selfUpdateTimeout)
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

	// Self-update (ADR 0004) is independent of source polling: a deployed build
	// checks immediately and then on its own cadence with a short deadline. A
	// "dev" local build always skips self-update — there is no point swapping a
	// `go run` temp binary, and it lets local development run without hitting
	// the GitHub API. Use -self-update=false to disable it for a release-like
	// local build.
	var updateMonitor *rfs.UpdateMonitor
	if shouldEnableSelfUpdate(version, *selfUpdate) {
		updateMonitor, err = rfs.NewUpdateMonitor(version, rfs.UpdateMonitorConfig{
			CheckInterval: *selfUpdateInterval,
			CheckTimeout:  *selfUpdateTimeout,
		})
		if err != nil {
			log.Printf("self-update disabled: %v", err)
		} else {
			log.Printf("self-update enabled: current version %s; checking every %s with a %s timeout", version, *selfUpdateInterval, *selfUpdateTimeout)
		}
	} else if version == "dev" {
		log.Printf("self-update disabled: dev build")
	} else {
		log.Printf("self-update disabled: -self-update=false")
	}

	log.Printf("serving feeds on %s", *addr)
	logFeeds(*addr, registeredSources)
	log.Printf("poll schedule: first cycle starts immediately, then every %s (%s)", *interval, pollCycleDetails(registeredSources))

	// reexec is closed when a self-update is applied, signalling main to drain
	// (via stop(), the same path a SIGTERM takes) and then syscall.Exec the
	// freshly-swapped binary in place. sync.Once guards against double-close.
	reexec := make(chan struct{})
	var reexecOnce sync.Once
	var reexecPath string
	requestReexec := func(path string) {
		reexecOnce.Do(func() {
			reexecPath = path
			close(reexec)
		})
	}
	if updateMonitor != nil {
		go func() {
			for event := range updateMonitor.Run(ctx) {
				if event.Err != nil {
					log.Printf("self-update: %v", event.Err)
				} else if event.Result.Latest == "" {
					log.Printf("self-update: no releases found; current %s", event.Result.Current)
				} else if event.Result.Applied {
					log.Printf("self-update: installed release %s over current %s; draining and re-executing", event.Result.Latest, event.Result.Current)
					requestReexec(event.ReexecPath)
				} else {
					log.Printf("self-update: latest release %s, current %s; no update", event.Result.Latest, event.Result.Current)
				}
			}
		}()
	}

	loop := rfs.Loop{
		Poll: func(ctx context.Context) {
			started := time.Now()
			pollAll(ctx, registeredSources, poller, gate)
			logNextPollCycle(started, *interval, registeredSources)
		},
		Interval: *interval,
	}
	loopDone := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(loopDone)
	}()

	// A self-update applies by triggering the same graceful shutdown a signal
	// does: stop() cancels ctx, the loop drains its in-flight poll, the
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
