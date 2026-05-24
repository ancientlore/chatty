package meshmtr

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string) *Client {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{},
	}
}

func (c *Client) get(path string) (any, error) {
	url := c.BaseURL + strings.TrimPrefix(path, "/")
	req, err := http.NewRequest("GET", url, nil)
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

func NewTools(baseURL, token string) ([]tool.Tool, error) {
	client := NewClient(baseURL, token)

	tools := []tool.Tool{}

	infoTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_info",
			Description: "Get general information about the Meshtastic network and node.",
		},
		func(ctx tool.Context, args EmptyArgs) (any, error) {
			return client.get("") // Root endpoint is info
		},
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, infoTool)

	nodesTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_nodes",
			Description: "Get a list of all visible nodes on the Meshtastic network.",
		},
		func(ctx tool.Context, args EmptyArgs) (any, error) {
			return client.get("nodes")
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
			return client.get("channels")
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
			return client.get("telemetry")
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
			return client.get("network")
		},
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, networkTool)

	return tools, nil
}
