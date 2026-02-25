package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"proxy-checker/internal/config"
)

// ProxyChecker handles proxy validation and protocol detection
type ProxyChecker struct {
	testURLs   []string
	timeout    time.Duration
	httpClient *http.Client
}

// NewProxyChecker creates a new proxy checker
func NewProxyChecker(cfg *config.Config) *ProxyChecker {
	return &ProxyChecker{
		testURLs: cfg.TestURLs,
		timeout:  cfg.CheckTimeout,
	}
}

// CheckProxy checks a single proxy and returns the result
func (c *ProxyChecker) CheckProxy(job ProxyJob) ProxyResult {
	result := ProxyResult{
		ID:      job.ID,
		Address: job.Address,
		Alive:   false,
	}

	timeout := c.timeout
	if job.Timeout > 0 {
		timeout = job.Timeout
	}

	// Try protocols in order: HTTP -> HTTPS -> SOCKS5 -> SOCKS4
	protocols := []struct {
		name string
		test func(string, time.Duration) (bool, int, error)
	}{
		{"http", c.testHTTP},
		{"https", c.testHTTPS},
		{"socks5", c.testSOCKS5},
		{"socks4", c.testSOCKS4},
	}

	for _, proto := range protocols {
		alive, latency, err := proto.test(job.Address, timeout)
		if alive {
			result.Alive = true
			result.Type = proto.name
			result.Latency = latency
			return result
		}
		if err != nil {
			result.Error = err.Error()
		}
	}

	result.Type = "unknown"
	return result
}

// testHTTP tests proxy as HTTP proxy
func (c *ProxyChecker) testHTTP(address string, timeout time.Duration) (bool, int, error) {
	proxyURL, err := url.Parse(fmt.Sprintf("http://%s", address))
	if err != nil {
		return false, 0, err
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return c.testWithClient(client)
}

// testHTTPS tests proxy as HTTPS proxy (CONNECT method)
func (c *ProxyChecker) testHTTPS(address string, timeout time.Duration) (bool, int, error) {
	// HTTPS proxies use the same CONNECT method as HTTP
	// The difference is in the target URL
	return c.testHTTP(address, timeout)
}

// testSOCKS5 tests proxy as SOCKS5 proxy
func (c *ProxyChecker) testSOCKS5(address string, timeout time.Duration) (bool, int, error) {
	proxyURL, err := url.Parse(fmt.Sprintf("socks5://%s", address))
	if err != nil {
		return false, 0, err
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return c.testWithClient(client)
}

// testSOCKS4 tests proxy as SOCKS4 proxy
func (c *ProxyChecker) testSOCKS4(address string, timeout time.Duration) (bool, int, error) {
	proxyURL, err := url.Parse(fmt.Sprintf("socks4://%s", address))
	if err != nil {
		return false, 0, err
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return c.testWithClient(client)
}

// testWithClient makes a request using the configured client
func (c *ProxyChecker) testWithClient(client *http.Client) (bool, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
	defer cancel()

	start := time.Now()

	// Try each test URL until one succeeds
	var lastErr error
	for _, testURL := range c.testURLs {
		req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		latency := int(time.Since(start).Milliseconds())

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return true, latency, nil
		}

		lastErr = fmt.Errorf("status code: %d", resp.StatusCode)
	}

	return false, int(time.Since(start).Milliseconds()), lastErr
}

// ParseAddress validates and normalizes a proxy address
func ParseAddress(addr string) (string, bool) {
	addr = strings.TrimSpace(addr)

	// Remove protocol prefix if present
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimPrefix(addr, "socks4://")
	addr = strings.TrimPrefix(addr, "socks5://")

	// Basic validation: should contain : and have reasonable length
	if !strings.Contains(addr, ":") || len(addr) < 9 || len(addr) > 50 {
		return "", false
	}

	// Split and validate IP:port format
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", false
	}

	// Validate port is numeric
	port := parts[1]
	for _, c := range port {
		if c < '0' || c > '9' {
			return "", false
		}
	}

	return addr, true
}
