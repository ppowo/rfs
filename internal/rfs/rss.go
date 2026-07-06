package rfs

import (
	"encoding/xml"
	"time"
)

// RenderRSS renders Source metadata and Items as an RSS 2.0 document.
func RenderRSS(meta SourceMeta, items []Item) ([]byte, error) {
	doc := rssDocument{
		Version: "2.0",
		Channel: rssChannel{
			Title:       meta.Title,
			Description: meta.Description,
			Link:        meta.Link,
			Items:       make([]rssItem, 0, len(items)),
		},
	}

	for _, item := range items {
		doc.Channel.Items = append(doc.Channel.Items, rssItem{
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			GUID: rssGUID{
				IsPermaLink: false,
				Value:       item.GUID,
			},
			PubDate: formatRSSDate(item.PubDate),
		})
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

func formatRSSDate(t time.Time) string {
	return t.UTC().Format(time.RFC1123Z)
}

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	Description string  `xml:"description"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}
