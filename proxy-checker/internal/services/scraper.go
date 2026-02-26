package services

import (
	"regexp"
	"time"

	"github.com/go-resty/resty/v2"
)

// Scraper handles fetching and parsing proxy lists from URLs
type Scraper struct {
	client *resty.Client
}

// NewScraper creates a new scraper
func NewScraper() *Scraper {
	client := resty.New().
		SetTimeout(30 * time.Second).
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	return &Scraper{client: client}
}

// Fetch retrieves and parses proxies from a URL
func (s *Scraper) Fetch(url string) ([]string, error) {
	resp, err := s.client.R().Get(url)
	if err != nil {
		return nil, err
	}

	body := resp.String()
	return s.ParseProxies(body), nil
}

// FetchResult contains the result of fetching a source
type FetchResult struct {
	URL         string
	Proxies     []string
	RawCount    int // Total proxies found in response (before dedup)
	Error       error
}

// ParseProxies extracts proxy addresses from text content
func (s *Scraper) ParseProxies(body string) []string {
	// Regex pattern for IP:PORT
	// Matches: 192.168.1.1:8080, 10.0.0.1:3128, etc.
	pattern := `(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):(\d{1,5})`

	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(body, -1)

	// Use map for deduplication
	seen := make(map[string]bool)
	var proxies []string

	for _, match := range matches {
		if len(match) >= 3 {
			addr := match[1] + ":" + match[2]
			if !seen[addr] {
				seen[addr] = true
				// Validate using ParseAddress
				if parsed, ok := ParseAddress(addr); ok {
					proxies = append(proxies, parsed)
				}
			}
		}
	}

	return proxies
}

// FetchMultiple fetches proxies from multiple URLs
func (s *Scraper) FetchMultiple(urls []string) map[string][]string {
	results := make(map[string][]string)

	for _, u := range urls {
		proxies, err := s.Fetch(u)
		if err != nil {
			results[u] = nil
			continue
		}
		results[u] = proxies
	}

	return results
}
