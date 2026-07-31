package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	appconfig "linde-go-v2/internal/config"
)

const defaultTimeout = 30 * time.Second

type ToolConfig struct {
	BaseURL    string
	SiteID     int
	UserID     int
	Timeout    time.Duration
	Headers    map[string]string
	HTTPClient *http.Client
}

type client struct {
	baseURL    string
	defaultSID int
	headers    map[string]string
	httpClient *http.Client
}

type basicResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
}

func ToolConfigFromAppConfig(cfg *appconfig.AppConfig) ToolConfig {
	if cfg == nil {
		return ToolConfig{}
	}

	headers := make(map[string]string, len(cfg.WorkPermitAPI.Headers))
	for key, value := range cfg.WorkPermitAPI.Headers {
		headers[key] = value
	}

	return ToolConfig{
		BaseURL: cfg.WorkPermitAPI.BaseURL,
		SiteID:  cfg.WorkPermitAPI.SiteID,
		UserID:  cfg.WorkPermitAPI.UserID,
		Timeout: cfg.WorkPermitAPI.Timeout,
		Headers: headers,
	}
}

func newClient(cfg ToolConfig) (*client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	headers := make(map[string]string, len(cfg.Headers))
	for key, value := range cfg.Headers {
		headers[key] = value
	}

	return &client{
		baseURL:    baseURL,
		defaultSID: cfg.SiteID,
		headers:    headers,
		httpClient: httpClient,
	}, nil
}

func (c *client) get(ctx context.Context, path string, query url.Values) (basicResponse, error) {
	var response basicResponse

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return response, fmt.Errorf("build GET request %s: %w", path, err)
	}

	req.URL.RawQuery = query.Encode()
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	if err := c.do(req, &response); err != nil {
		return response, err
	}

	return response, nil
}

func (c *client) post(ctx context.Context, path string, body any) (basicResponse, error) {
	var response basicResponse

	payload, err := json.Marshal(body)
	if err != nil {
		return response, fmt.Errorf("marshal POST body for %s: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return response, fmt.Errorf("build POST request %s: %w", path, err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	if err := c.do(req, &response); err != nil {
		return response, err
	}

	return response, nil
}

func (c *client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s failed: %w", req.Method, req.URL.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request %s %s returned status %d: %s", req.Method, req.URL.String(), resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}

func (c *client) resolveSiteID(siteID int) (int, error) {
	if siteID != 0 {
		return siteID, nil
	}
	if c.defaultSID != 0 {
		return c.defaultSID, nil
	}
	return 0, fmt.Errorf("siteid is required")
}
