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

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	interval := flag.Duration("interval", time.Hour, "global source polling interval")
	domainSpacing := flag.Duration("domain-spacing", 10*time.Second, "minimum spacing between requests to the same domain")
	dbPath := flag.String("db", "", "SQLite database path; defaults to the OS user cache dir, or use :memory:")
	flag.Parse()
	if *interval <= 0 {
		log.Fatalf("interval must be positive, got %s", *interval)
	}
	if *domainSpacing < 0 {
		log.Fatalf("domain-spacing must not be negative, got %s", *domainSpacing)
	}

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

	log.Printf("serving feeds on %s", *addr)
	logFeeds(*addr, registeredSources)

	go pollLoop(ctx, registeredSources, poller, gate, *interval)

	server := &http.Server{
		Addr:    *addr,
		Handler: rfs.NewHTTPHandler(store, registeredSources),
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
}

func pollLoop(ctx context.Context, sources []rfs.Source, poller rfs.Poller, gate *rfs.DomainGate, interval time.Duration) {
	pollAll(ctx, sources, poller, gate)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollAll(ctx, sources, poller, gate)
		}
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
