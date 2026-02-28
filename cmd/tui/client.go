package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Entry struct {
	Pod        string  `json:"pod"`
	Container  string  `json:"container"`
	SecretPath string  `json:"secret_path"`
	ReadPerSec float64 `json:"reads_per_sec"`
	LastRead   string  `json:"last_read"`
	Cached     bool    `json:"cached"`
}

type APIResponse struct {
	Timestamp              string  `json:"timestamp"`
	ObservationWindowSecs  int     `json:"observation_window_seconds"`
	Entries                []Entry `json:"entries"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) FetchEntries() (*APIResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/secret-access")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}
	return &result, nil
}