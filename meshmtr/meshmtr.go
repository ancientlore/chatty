package meshmtr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"google.golang.org/adk/tool"
)

type Client struct {
	BaseURL    string
	Token      string
	SourceName string
	SourceID   string
	mu         sync.Mutex
	HTTP       *http.Client
}

func NewClient(baseURL, token, sourceName string) *Client {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		SourceName: sourceName,
		HTTP:       &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

var tracer = otel.Tracer("meshmtr")

func (c *Client) resolveSourceID(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.SourceID != "" {
		return nil
	}

	url := c.BaseURL + "sources"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create sources request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute sources request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sources API error %d: %s", resp.StatusCode, string(body))
	}

	var sources sourcesResponse
	if err := json.NewDecoder(resp.Body).Decode(&sources); err != nil {
		return fmt.Errorf("failed to decode sources response: %w", err)
	}

	if !sources.Success {
		return fmt.Errorf("fetching sources failed according to API response")
	}

	for _, s := range sources.Data {
		if c.SourceName != "" {
			if strings.EqualFold(s.Name, c.SourceName) {
				c.SourceID = s.ID
				break
			}
		} else {
			if s.IsPrimary {
				c.SourceID = s.ID
				break
			}
		}
	}

	if c.SourceID == "" {
		if c.SourceName != "" {
			slog.Error("Source not found", "source", c.SourceName)
			return fmt.Errorf("source %q not found", c.SourceName)
		}
		slog.Error("No primary source found")
		return fmt.Errorf("no primary source found")
	}

	slog.Info("Source ID resolved", "source_id", c.SourceID)
	return nil
}

func (c *Client) get(ctx context.Context, path string, target any) error {
	if err := c.resolveSourceID(ctx); err != nil {
		return fmt.Errorf("failed to resolve source ID: %w", err)
	}

	url := c.BaseURL + fmt.Sprintf("sources/%s/", c.SourceID) + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

type EmptyArgs struct{}

type sourcesResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		IsPrimary bool   `json:"isPrimary"`
	} `json:"data"`
}

func NewTools(baseURL, token, sourceName string) ([]tool.Tool, error) {
	client := NewClient(baseURL, token, sourceName)

	tools := []tool.Tool{}

	nodesTool, err := newNodesTool(client, true, 1)
	if err != nil {
		return nil, err
	}
	tools = append(tools, nodesTool)

	channelsTool, err := newChannelsTool(client)
	if err != nil {
		return nil, err
	}
	tools = append(tools, channelsTool)

	since := time.Now().Add(-4 * time.Hour).UnixMilli()
	telemetryTool, err := newTelemetryTool(client, 200, 0, 0, since, "")
	if err != nil {
		return nil, err
	}
	tools = append(tools, telemetryTool)

	networkTool, err := newNetworkTool(client)
	if err != nil {
		return nil, err
	}
	tools = append(tools, networkTool)

	return tools, nil
}
