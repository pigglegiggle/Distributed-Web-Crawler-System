package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// CrawlResult is persisted as JSON for each successfully crawled page.
type CrawlResult struct {
	URL         string    `json:"url"`
	Depth       int       `json:"depth"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Links       []string  `json:"links"`
}

type Crawler struct {
	client *http.Client
}

func NewCrawler(timeout time.Duration) *Crawler {
	return &Crawler{
		client: &http.Client{Timeout: timeout},
	}
}

func (c *Crawler) FetchAndExtract(ctx context.Context, rawURL string) (*CrawlResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "educational-distributed-crawler/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	title, desc, links := extractMetadata(doc, rawURL)
	return &CrawlResult{
		URL:         rawURL,
		Title:       title,
		Description: desc,
		Timestamp:   time.Now().UTC(),
		Links:       links,
	}, nil
}

func extractMetadata(doc *html.Node, baseRawURL string) (string, string, []string) {
	baseURL, _ := url.Parse(baseRawURL)
	linkSet := map[string]struct{}{}
	var title string
	var description string

	var visit func(n *html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if n.FirstChild != nil && title == "" {
					title = strings.TrimSpace(n.FirstChild.Data)
				}
			case "meta":
				var name, content string
				for _, a := range n.Attr {
					if strings.EqualFold(a.Key, "name") {
						name = strings.ToLower(strings.TrimSpace(a.Val))
					}
					if strings.EqualFold(a.Key, "content") {
						content = strings.TrimSpace(a.Val)
					}
				}
				if name == "description" && description == "" {
					description = content
				}
			case "a":
				for _, a := range n.Attr {
					if a.Key != "href" {
						continue
					}
					norm := normalizeURL(baseURL, a.Val)
					if norm != "" {
						linkSet[norm] = struct{}{}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}

	visit(doc)
	links := make([]string, 0, len(linkSet))
	for l := range linkSet {
		links = append(links, l)
	}
	return title, description, links
}

func normalizeURL(baseURL *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if baseURL != nil {
		u = baseURL.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	u.Fragment = ""
	return u.String()
}
