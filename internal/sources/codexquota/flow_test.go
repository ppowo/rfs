package codexquota_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ppowo/rfs/internal/rfs"
	"github.com/ppowo/rfs/internal/sources/codexquota"
)

func TestFlowExtractsAlertEventsFromForecastJSON(t *testing.T) {
	page, err := os.ReadFile("testdata/forecast.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	items, err := (codexquota.Flow{}).Extract(rfs.Page(page))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	likelihood := items[0]
	if likelihood.GUID != "codex-quota-reset:likelihood:2026-07-15T05:59:46Z" {
		t.Errorf("likelihood GUID = %q", likelihood.GUID)
	}
	if likelihood.Title != "Codex reset likelihood reached 89%" {
		t.Errorf("likelihood title = %q", likelihood.Title)
	}
	if likelihood.Link != codexquota.PageURL {
		t.Errorf("likelihood link = %q", likelihood.Link)
	}
	wantLikelihoodDescription := "Forecast rose from 3% to 89%. Signals: LLM tweet-context judgment (+77), OpenAI team vagueposting (+14)."
	if likelihood.Description != wantLikelihoodDescription {
		t.Errorf("likelihood description = %q, want %q", likelihood.Description, wantLikelihoodDescription)
	}
	assertDate(t, likelihood.PubDate, "2026-07-15T05:59:46Z")

	reset := items[1]
	if reset.GUID != "codex-quota-reset:reset:2077114635308986427" {
		t.Errorf("reset GUID = %q", reset.GUID)
	}
	if reset.Title != "Codex quota reset confirmed" {
		t.Errorf("reset title = %q", reset.Title)
	}
	if reset.Link != "https://x.com/thsottiaux/status/2077114635308986427" {
		t.Errorf("reset link = %q", reset.Link)
	}
	wantResetDescription := "We are once again resetting the usage limits for all.\n\nWhy it counted: Explicit completed Codex quota-reset post."
	if reset.Description != wantResetDescription {
		t.Errorf("reset description = %q, want %q", reset.Description, wantResetDescription)
	}
	assertDate(t, reset.PubDate, "2026-07-14T19:34:54Z")
}

func TestFlowExtractsPendingResetAnnouncement(t *testing.T) {
	page := rfs.Page(`{
		"history": [],
		"tiboPosts": [{
			"guid": "pending-123",
			"pubDate": "2026-07-16T08:30:00Z",
			"title": "We will reset Codex usage limits within 24 hours.",
			"tweetAssessment": {
				"category": "reset_announced",
				"reason": "Explicit upcoming reset announcement."
			}
		}]
	}`)

	items, err := (codexquota.Flow{}).Extract(page)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].GUID != "codex-quota-reset:reset:pending-123" {
		t.Errorf("GUID = %q", items[0].GUID)
	}
	if items[0].Title != "Codex quota reset announced" {
		t.Errorf("title = %q", items[0].Title)
	}
	if items[0].Link != codexquota.PageURL {
		t.Errorf("fallback link = %q", items[0].Link)
	}
}

func TestFlowOnlyAlertsWhenLikelihoodCrossesThreshold(t *testing.T) {
	page := rfs.Page(`{
		"history": [
			{"at": "2026-07-16T01:00:00Z", "fromScore": 69, "toScore": 70},
			{"at": "2026-07-16T02:00:00Z", "fromScore": 70, "toScore": 90},
			{"at": "2026-07-16T03:00:00Z", "fromScore": 45, "toScore": 69},
			{"at": "2026-07-16T04:00:00Z", "fromScore": 90, "toScore": 10}
		],
		"tiboPosts": []
	}`)

	items, err := (codexquota.Flow{}).Extract(page)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Title != "Codex reset likelihood reached 70%" {
		t.Errorf("title = %q", items[0].Title)
	}
}

func TestFlowRejectsUnusableForecasts(t *testing.T) {
	tests := []struct {
		name    string
		page    rfs.Page
		wantErr string
	}{
		{name: "invalid JSON", page: rfs.Page(`{`), wantErr: "decode forecast"},
		{name: "missing history", page: rfs.Page(`{"tiboPosts": []}`), wantErr: "missing history"},
		{name: "missing posts", page: rfs.Page(`{"history": []}`), wantErr: "missing tiboPosts"},
		{
			name:    "threshold event missing fromScore",
			page:    rfs.Page(`{"history": [{"at": "2026-07-16T01:00:00Z", "toScore": 70}], "tiboPosts": []}`),
			wantErr: "missing fromScore",
		},
		{
			name:    "threshold event missing toScore",
			page:    rfs.Page(`{"history": [{"at": "2026-07-16T01:00:00Z", "fromScore": 69}], "tiboPosts": []}`),
			wantErr: "missing toScore",
		},
		{
			name:    "threshold event missing date",
			page:    rfs.Page(`{"history": [{"fromScore": 69, "toScore": 70}], "tiboPosts": []}`),
			wantErr: "missing publication date",
		},
		{
			name:    "reset event missing guid",
			page:    rfs.Page(`{"history": [], "tiboPosts": [{"pubDate": "2026-07-16T01:00:00Z", "tweetAssessment": {"category": "reset_completed"}}]}`),
			wantErr: "missing guid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (codexquota.Flow{}).Extract(test.page)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func assertDate(t *testing.T, got *time.Time, want string) {
	t.Helper()
	if got == nil {
		t.Fatal("publication date is nil")
	}
	if got.Format(time.RFC3339Nano) != want {
		t.Errorf("publication date = %s, want %s", got.Format(time.RFC3339Nano), want)
	}
}
