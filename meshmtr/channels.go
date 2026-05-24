package meshmtr

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type ChannelData struct {
	ID                int    `json:"id" jsonschema:"Channel index"`
	Name              string `json:"name" jsonschema:"The raw name of the channel"`
	DisplayName       string `json:"displayName" jsonschema:"The human-readable display name of the channel"`
	Role              int    `json:"role" jsonschema:"Numeric role identifier"`
	RoleName          string `json:"roleName" jsonschema:"String representation of the role (e.g. Primary, Secondary)"`
	UplinkEnabled     bool   `json:"uplinkEnabled" jsonschema:"Whether uplink is enabled"`
	DownlinkEnabled   bool   `json:"downlinkEnabled" jsonschema:"Whether downlink is enabled"`
	PositionPrecision int    `json:"positionPrecision" jsonschema:"The position precision"`
	PskSet            bool   `json:"pskSet" jsonschema:"Whether a Pre-Shared Key (PSK) is set"`
	EncryptionStatus  string `json:"encryptionStatus" jsonschema:"Status of encryption (e.g. default, secure)"`
}

type ChannelsResponse struct {
	Success bool          `json:"success"`
	Count   int           `json:"count"`
	Data    []ChannelData `json:"data"`
}

func newChannelsTool(client *Client) (tool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_channels",
			Description: "Get information about the configured channels on the local node, including the name, role, channel ID, position precision, and encryption status.",
		},
		func(ctx tool.Context, args EmptyArgs) (*ChannelsResponse, error) {
			tctx, span := tracer.Start(ctx, "meshmtr.get_mesh_channels")
			defer span.End()
			var resp ChannelsResponse
			if err := client.get(tctx, "channels", &resp); err != nil {
				return nil, err
			}
			return &resp, nil
		},
	)
}
