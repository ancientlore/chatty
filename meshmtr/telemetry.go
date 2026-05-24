package meshmtr

import (
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type TelemetryData struct {
	ID              int     `json:"id" jsonschema:"Telemetry entry ID"`
	NodeId          string  `json:"nodeId" jsonschema:"The node ID string"`
	NodeNum         uint32  `json:"nodeNum" jsonschema:"The node number"`
	TelemetryType   string  `json:"telemetryType" jsonschema:"The type of telemetry, e.g. batteryLevel, voltage, channelUtilization, airUtilTx"`
	Timestamp       int64   `json:"timestamp" jsonschema:"Timestamp of telemetry"`
	Value           float64 `json:"value" jsonschema:"The telemetry value"`
	Unit            string  `json:"unit" jsonschema:"The unit of measurement"`
	CreatedAt       int64   `json:"createdAt" jsonschema:"When the telemetry was created in the system"`
	PacketTimestamp *int64  `json:"packetTimestamp" jsonschema:"Timestamp of the packet"`
	PacketId        *uint32 `json:"packetId" jsonschema:"The ID of the packet"`
	Channel         *int    `json:"channel" jsonschema:"Channel index if applicable"`
	PrecisionBits   *int    `json:"precisionBits" jsonschema:"Precision bits if applicable"`
	GpsAccuracy     *int    `json:"gpsAccuracy" jsonschema:"GPS accuracy if applicable"`
	SourceId        string  `json:"sourceId" jsonschema:"The source network ID"`
}

type TelemetryResponse struct {
	Success bool            `json:"success"`
	Count   int             `json:"count"`
	Offset  int             `json:"offset"`
	Limit   int             `json:"limit"`
	Data    []TelemetryData `json:"data"`
}

type TelemetryArgs struct {
	NodeID string `json:"nodeId" jsonschema:"Meshtastic node ID, e.g., !a1b2c3d4"`
}

func newTelemetryTool(client *Client, limit int, offset int, before int64, since int64, telemetryType string) (tool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_telemetry",
			Description: "Get telemetry data such as air utilization, battery levels, and environmental data for a specific node on the network.",
		},
		func(ctx tool.Context, args TelemetryArgs) (*TelemetryResponse, error) {
			tctx, span := tracer.Start(ctx, "meshmtr.get_mesh_telemetry")
			defer span.End()
			var resp TelemetryResponse

			path := fmt.Sprintf("telemetry/%s?limit=%d&offset=%d", args.NodeID, limit, offset)
			if before != 0 {
				path += fmt.Sprintf("&before=%d", before)
			}
			if since != 0 {
				path += fmt.Sprintf("&since=%d", since)
			}
			if telemetryType != "" {
				path += fmt.Sprintf("&type=%s", telemetryType)
			}

			if err := client.get(tctx, path, &resp); err != nil {
				return nil, err
			}
			return &resp, nil
		},
	)
}
