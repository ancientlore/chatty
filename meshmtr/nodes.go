package meshmtr

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type NodeData struct {
	NodeNum              uint32   `json:"nodeNum" jsonschema:"The node number"`
	NodeId               string   `json:"nodeId" jsonschema:"The node ID string"`
	LongName             string   `json:"longName" jsonschema:"The long name of the node"`
	ShortName            string   `json:"shortName" jsonschema:"The short name of the node"`
	HwModel              int      `json:"hwModel" jsonschema:"Hardware model ID"`
	Role                 int      `json:"role" jsonschema:"Role of the node"`
	HopsAway             *int     `json:"hopsAway" jsonschema:"Number of hops away"`
	LastMessageHops      *int     `json:"lastMessageHops" jsonschema:"Number of hops for the last message"`
	ViaMqtt              *bool    `json:"viaMqtt" jsonschema:"Whether the node is heard via MQTT"`
	IsStoreForwardServer *bool    `json:"isStoreForwardServer" jsonschema:"Whether it's a store and forward server"`
	Macaddr              *string  `json:"macaddr" jsonschema:"MAC address if known"`
	Latitude             *float64 `json:"latitude" jsonschema:"Latitude of the node"`
	Longitude            *float64 `json:"longitude" jsonschema:"Longitude of the node"`
	Altitude             *int     `json:"altitude" jsonschema:"Altitude in meters"`
	BatteryLevel         *int     `json:"batteryLevel" jsonschema:"Battery percentage"`
	Voltage              *float64 `json:"voltage" jsonschema:"Voltage of the node"`
	ChannelUtilization   *float64 `json:"channelUtilization" jsonschema:"Channel utilization percentage"`
	AirUtilTx            *float64 `json:"airUtilTx" jsonschema:"Air utilization TX percentage"`
	LastHeard            int64    `json:"lastHeard" jsonschema:"Timestamp when last heard"`
	Snr                  *float64 `json:"snr" jsonschema:"Signal-to-noise ratio"`
	Rssi                 *int     `json:"rssi" jsonschema:"Received signal strength indicator"`
	FirmwareVersion      *string  `json:"firmwareVersion" jsonschema:"Firmware version"`
	Channel              int      `json:"channel" jsonschema:"Primary channel index"`
	IsFavorite           bool     `json:"isFavorite" jsonschema:"Whether the node is a favorite"`
	Mobile               *int     `json:"mobile" jsonschema:"Mobile status"`
	RebootCount          *int     `json:"rebootCount" jsonschema:"Reboot count"`
	TimeOffsetSeconds    *int     `json:"timeOffsetSeconds" jsonschema:"Time offset in seconds"`
	UptimeSeconds        *int     `json:"uptimeSeconds" jsonschema:"Uptime in seconds"`
}

type NodesResponse struct {
	Success bool       `json:"success"`
	Count   int        `json:"count"`
	Data    []NodeData `json:"data"`
}

type nodeCache struct {
	mu        sync.RWMutex
	data      *NodesResponse
	expiresAt time.Time
}

type NodesArgs struct {
	Search *string `json:"search,omitempty" jsonschema:"Search string to filter nodes by short name, node ID, or partial long name/short name. Use this to find a node's unique hex nodeId (e.g., '!a1b2c3d4') when you only have its short or long name."`
}

func newNodesTool(client *Client, active bool, sinceDays int) (tool.Tool, error) {
	cache := &nodeCache{}

	return functiontool.New(
		functiontool.Config{
			Name:        "get_mesh_nodes",
			Description: "Get a list of visible nodes on the Meshtastic network. Can optionally search by node short name, long name, or node ID. Use this tool first to resolve short or long names into unique Node IDs before fetching telemetry.",
		},
		func(ctx tool.Context, args NodesArgs) (*NodesResponse, error) {
			tctx, span := tracer.Start(ctx, "meshmtr.get_mesh_nodes")
			defer span.End()

			var cachedData *NodesResponse
			cache.mu.RLock()
			if cache.data != nil && time.Now().Before(cache.expiresAt) {
				cachedData = cache.data
			}
			cache.mu.RUnlock()

			if cachedData == nil {
				cache.mu.Lock()
				if cache.data != nil && time.Now().Before(cache.expiresAt) {
					cachedData = cache.data
				} else {
					var resp NodesResponse
					path := fmt.Sprintf("nodes?active=%t&sinceDays=%d", active, sinceDays)
					if err := client.get(tctx, path, &resp); err != nil {
						cache.mu.Unlock()
						return nil, err
					}
					cache.data = &resp
					cache.expiresAt = time.Now().Add(15 * time.Minute)
					cachedData = cache.data
				}
				cache.mu.Unlock()
			}

			if args.Search == nil || *args.Search == "" {
				return cachedData, nil
			}

			search := strings.ToLower(strings.TrimPrefix(*args.Search, "!"))
			var pass1, pass2, pass3 []NodeData

			for _, n := range cachedData.Data {
				shortName := strings.ToLower(n.ShortName)
				nodeID := strings.ToLower(strings.TrimPrefix(n.NodeId, "!"))
				longName := strings.ToLower(n.LongName)

				if shortName == search {
					pass1 = append(pass1, n)
				} else if nodeID == search {
					pass2 = append(pass2, n)
				} else if strings.Contains(longName, search) || strings.Contains(shortName, search) {
					pass3 = append(pass3, n)
				}
			}

			results := append(pass1, pass2...)
			results = append(results, pass3...)

			return &NodesResponse{
				Success: true,
				Count:   len(results),
				Data:    results,
			}, nil
		},
	)
}
