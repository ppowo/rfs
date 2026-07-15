package codexquota

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ppowo/rfs/internal/rfs"
)

const (
	PageURL        = "https://www.willcodexquotareset.com/"
	ForecastURL    = PageURL + "api/forecast"
	ExtractVersion = 1

	likelihoodThreshold = 70
)

type Flow struct{}

func (Flow) Version() int { return ExtractVersion }

func (Flow) Extract(page rfs.Page) ([]rfs.ExtractedItem, error) {
	var response forecastResponse
	if err := json.Unmarshal(page, &response); err != nil {
		return nil, fmt.Errorf("codex quota: decode forecast: %w", err)
	}
	if response.History == nil {
		return nil, errors.New("codex quota: forecast is missing history")
	}
	if response.TiboPosts == nil {
		return nil, errors.New("codex quota: forecast is missing tiboPosts")
	}

	items := make([]rfs.ExtractedItem, 0)
	seenGUIDs := make(map[string]struct{})

	// Only source-provided transition timestamps become item identities. fetchedAt
	// changes on every refresh and would turn an unchanged forecast into RSS spam.

	for index, event := range response.History {
		if event.FromScore == nil {
			return nil, fmt.Errorf("codex quota: history[%d] is missing fromScore", index)
		}
		if event.ToScore == nil {
			return nil, fmt.Errorf("codex quota: history[%d] is missing toScore", index)
		}
		fromScore := *event.FromScore
		toScore := *event.ToScore
		if fromScore >= likelihoodThreshold || toScore < likelihoodThreshold {
			continue
		}
		published, err := parseDate(event.At)
		if err != nil {
			return nil, fmt.Errorf("codex quota: history[%d] threshold event: %w", index, err)
		}
		guid := "codex-quota-reset:likelihood:" + published.Format(time.RFC3339Nano)
		if _, exists := seenGUIDs[guid]; exists {
			continue
		}
		seenGUIDs[guid] = struct{}{}
		items = append(items, rfs.ExtractedItem{
			GUID:        guid,
			Title:       fmt.Sprintf("Codex reset likelihood reached %d%%", toScore),
			Link:        PageURL,
			Description: likelihoodDescription(event, fromScore, toScore),
			PubDate:     timePointer(published),
		})
	}

	for index, post := range response.TiboPosts {
		title, alert := resetAlertTitle(post.TweetAssessment.Category)
		if !alert {
			continue
		}
		if strings.TrimSpace(post.GUID) == "" {
			return nil, fmt.Errorf("codex quota: tiboPosts[%d] reset alert is missing guid", index)
		}
		published, err := parseDate(post.PubDate)
		if err != nil {
			return nil, fmt.Errorf("codex quota: tiboPosts[%d] reset alert: %w", index, err)
		}
		guid := "codex-quota-reset:reset:" + strings.TrimSpace(post.GUID)
		if _, exists := seenGUIDs[guid]; exists {
			continue
		}
		seenGUIDs[guid] = struct{}{}
		link := strings.TrimSpace(post.Link)
		if link == "" {
			link = PageURL
		}
		items = append(items, rfs.ExtractedItem{
			GUID:        guid,
			Title:       title,
			Link:        link,
			Description: resetDescription(post),
			PubDate:     timePointer(published),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].PubDate.After(*items[j].PubDate)
	})
	return items, nil
}

func likelihoodDescription(event historyEntry, fromScore, toScore int) string {
	description := fmt.Sprintf("Forecast rose from %d%% to %d%%.", fromScore, toScore)
	signals := make([]string, 0, len(event.Changes))
	for _, change := range event.Changes {
		label := strings.TrimSpace(change.Label)
		if label == "" {
			continue
		}
		signals = append(signals, fmt.Sprintf("%s (%+d)", label, change.Delta))
	}
	if len(signals) > 0 {
		description += " Signals: " + strings.Join(signals, ", ") + "."
	}
	return description
}

func resetAlertTitle(category string) (string, bool) {
	switch category {
	case "reset_announced":
		return "Codex quota reset announced", true
	case "reset_completed":
		return "Codex quota reset confirmed", true
	default:
		return "", false
	}
}

func resetDescription(post tiboPost) string {
	parts := make([]string, 0, 2)
	if text := strings.TrimSpace(post.Title); text != "" {
		parts = append(parts, text)
	}
	if reason := strings.TrimSpace(post.TweetAssessment.Reason); reason != "" {
		parts = append(parts, "Why it counted: "+reason)
	}
	if len(parts) == 0 {
		return "A Codex quota reset alert was published."
	}
	return strings.Join(parts, "\n\n")
}

func parseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("missing publication date")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse publication date %q: %w", value, err)
	}
	return parsed.UTC(), nil
}

func timePointer(value time.Time) *time.Time { return &value }

type forecastResponse struct {
	History   []historyEntry `json:"history"`
	TiboPosts []tiboPost     `json:"tiboPosts"`
}

type historyEntry struct {
	At        string        `json:"at"`
	FromScore *int          `json:"fromScore"`
	ToScore   *int          `json:"toScore"`
	Changes   []scoreChange `json:"changes"`
}

type scoreChange struct {
	Delta int    `json:"delta"`
	Label string `json:"label"`
}

type tiboPost struct {
	GUID            string          `json:"guid"`
	Link            string          `json:"link"`
	PubDate         string          `json:"pubDate"`
	Title           string          `json:"title"`
	TweetAssessment tweetAssessment `json:"tweetAssessment"`
}

type tweetAssessment struct {
	Category string `json:"category"`
	Reason   string `json:"reason"`
}
