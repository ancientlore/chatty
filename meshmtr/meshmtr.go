package meshmtr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type Client struct {
	BaseURL  string
	Token    string
	SourceID string
	HTTP     *http.Client
}

func NewClient(baseURL, token, sourceID string) *Client {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &Client{
		BaseURL:  baseURL,
		Token:    token,
		SourceID: sourceID,
		HTTP:     &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

var tracer = otel.Tracer("meshmtr")

func (c *Client) get(ctx context.Context, path string) (any, error) {
	var url string
	if c.SourceID != "" {
		url = c.BaseURL + fmt.Sprintf("sources/%s/", c.SourceID) + strings.TrimPrefix(path, "/")
	} else {
		url = c.BaseURL + strings.TrimPrefix(path, "/")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

type EmptyArgs struct{}

type sourcesResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

func NewTools(baseURL, token, sourceName string) ([]tool.Tool, error) {
	// 1. Create a temporary client without a SourceID to fetch the list of sources
	tempClient := NewClient(baseURL, token, "")
	rawSources, err := tempClient.get(context.Background(), "sources")
	if err != nil {
		slog.Error("Failed to fetch sources from MeshMonitor API", "error", err)
		return nil, fmt.Errorf("failed to fetch sources: %w", err)
	}

	// Convert map[string]interface{} (from any) to sourcesResponse structure
	rawBytes, err := json.Marshal(rawSources)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode sources: %w", err)
	}

	var sources sourcesResponse
	if err := json.Unmarshal(rawBytes, &sources); err != nil {
		return nil, fmt.Errorf("failed to parse sources: %w", err)
	}

	if !sources.Success {
		return nil, fmt.Errorf("fetching sources failed according to API response")
	}

	var sourceID string
	for _, s := range sources.Data {
		if strings.EqualFold(s.Name, sourceName) {
			sourceID = s.ID
			break
		}
	}

	if sourceID == "" {
		slog.Error("Specified source not found in MeshMonitor API", "source", sourceName)
		return nil, fmt.Errorf("source %q not found", sourceName)
	}

	client := NewClient(baseURL, token, sourceID)

	tools := []tool.Tool{}

	nodesTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_nodes",
			Description: "Get a list of all visible nodes on the Meshtastic network.",
		},
		func(ctx tool.Context, args EmptyArgs) (any, error) {
			tctx, span := tracer.Start(ctx, "meshmtr.get_mesh_nodes")
			defer span.End()
			return client.get(tctx, "nodes")
		},
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, nodesTool)

	channelsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_channels",
			Description: "Get information about the configured channels on the local node.",
		},
		func(ctx tool.Context, args EmptyArgs) (any, error) {
			tctx, span := tracer.Start(ctx, "meshmtr.get_mesh_channels")
			defer span.End()
			return client.get(tctx, "channels")
		},
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, channelsTool)

	telemetryTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_telemetry",
			Description: "Get telemetry data such as air utilization, battery levels, and environmental data for the local node and the network.",
		},
		func(ctx tool.Context, args EmptyArgs) (any, error) {
			tctx, span := tracer.Start(ctx, "meshmtr.get_mesh_telemetry")
			defer span.End()
			return client.get(tctx, "telemetry")
		},
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, telemetryTool)

	networkTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_network",
			Description: "Get network topology, routing information, and other network-level statistics.",
		},
		func(ctx tool.Context, args EmptyArgs) (any, error) {
			tctx, span := tracer.Start(ctx, "meshmtr.get_mesh_network")
			defer span.End()
			return client.get(tctx, "network")
		},
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, networkTool)

	return tools, nil
}
