package rfs

import (
	"context"
	"net/http"
	"strings"
)

type SnapshotReader interface {
	LoadSnapshot(context.Context, string) ([]Item, error)
}

type HTTPHandler struct {
	store   SnapshotReader
	sources map[string]Source
}

func NewHTTPHandler(store SnapshotReader, sources []Source) http.Handler {
	byID := make(map[string]Source, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}
	return HTTPHandler{store: store, sources: byID}
}

func (h HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sourceID, ok := sourceIDFromFeedPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	source, ok := h.sources[sourceID]
	if !ok {
		http.NotFound(w, r)
		return
	}

	items, err := h.store.LoadSnapshot(r.Context(), sourceID)
	if err != nil {
		http.Error(w, "load feed snapshot", http.StatusInternalServerError)
		return
	}
	body, err := RenderRSS(source.Meta, items)
	if err != nil {
		http.Error(w, "render feed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, _ = w.Write(body)
}

func sourceIDFromFeedPath(path string) (string, bool) {
	const prefix = "/feeds/"
	const suffix = ".xml"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	sourceID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if sourceID == "" || strings.Contains(sourceID, "/") {
		return "", false
	}
	return sourceID, true
}
