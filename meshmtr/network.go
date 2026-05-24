package meshmtr

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type NetworkData struct {
	TotalNodes      int   `json:"totalNodes" jsonschema:"Total number of nodes known to the network"`
	ActiveNodes     int   `json:"activeNodes" jsonschema:"Number of active nodes"`
	TracerouteCount int   `json:"tracerouteCount" jsonschema:"Number of traceroutes"`
	LastUpdated     int64 `json:"lastUpdated" jsonschema:"Timestamp of the last update"`
}

type NetworkResponse struct {
	Success bool        `json:"success"`
	Data    NetworkData `json:"data"`
}

func newNetworkTool(client *Client) (tool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_network",
			Description: "Get network-level statistics like the number of total and active nodes seen on the network. This tool does not return information about a specific node.",
		},
		func(ctx tool.Context, args EmptyArgs) (*NetworkResponse, error) {
			tctx, span := tracer.Start(ctx, "meshmtr.get_mesh_network")
			defer span.End()
			var resp NetworkResponse
			if err := client.get(tctx, "network", &resp); err != nil {
				return nil, err
			}
			return &resp, nil
		},
	)
}
