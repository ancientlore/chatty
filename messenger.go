package main

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
	"google.golang.org/adk/tool/geminitool"
	"google.golang.org/genai"

	"github.com/ancientlore/chatty/meshmtr"
)

func buildRunner(ctx context.Context, token, modelName, systemInstruction, meshAPIURL, meshAPIToken, meshSource string, meshAPITimeout time.Duration) (*runner.Runner, error) {
	// Initialize the genai client config
	clientConfig := &genai.ClientConfig{
		APIKey:  token,
		Backend: genai.BackendGeminiAPI,
	}

	// Create the Gemini model
	geminiModel, err := gemini.NewModel(ctx, modelName, clientConfig)
	if err != nil {
		return nil, err
	}

	const extraContext = `Perspective & Telemetry Rules:
- You (Gemma) are a chatbot running on the host MeshMonitor device.
- All telemetry, node list details, and network statistics retrieved by you via tools (or in the metadata below) are measured relative to YOUR device (the chatbot's node/antenna), NOT the user's device.
- For example, if a node is listed as 0 hops away in 'get_mesh_nodes' or in the metadata below, it means it is directly connected to YOU. Describe this as "0 hops from me" (or "directly connected to me"), NOT "0 hops from you".

Here is the real-time radio network telemetry for the active user/node who is currently sending you messages:
- Node ID: {node_id?} (The unique identifier of their device on the mesh network. Use this directly as the nodeId parameter for telemetry queries if they ask about 'my node', 'my battery', 'my signal', 'me', 'my device', etc.)
- Short Name: {short_name?} (The 4-character abbreviation of their device name)
- Long Name: {long_name?} (The full name of their device)
- Hops: {hops?} (The number of times the message was relayed to reach us; 0 means a direct connection)
- SNR: {snr?} (Signal-to-Noise Ratio in dB; higher is better, typically ranges from -20 to +10)
- RSSI: {rssi?} (Received Signal Strength Indicator in dBm; closer to 0 is better, e.g., -40 is excellent, -120 is very poor)
Here is some information about the network that is visible to you:
- Channel: {channel?} (The channel you are currently on, or "DM" if you are in a direct message)
- Node Count: {node_count?} (Total number of active nodes currently seen by your device on the mesh network)
- Direct Count: {direct_count?} (Number of nodes directly connected/visible to your device without relays)
`

	searchAgentCfg := llmagent.Config{
		Name:        "search_agent",
		Description: "An agent that can search the web for information.",
		Model:       geminiModel,
		Tools:       []tool.Tool{geminitool.GoogleSearch{}},
	}
	searchAgent, err := llmagent.New(searchAgentCfg)
	if err != nil {
		return nil, err
	}

	for _, t := range searchAgentCfg.Tools {
		slog.Info("Loaded subtool", "tool", t.Name())
	}

	tools := []tool.Tool{
		agenttool.New(searchAgent, nil),
	}
	if meshAPIURL != "" && meshAPIToken != "" {
		meshTools, err := meshmtr.NewTools(meshAPIURL, meshAPIToken, meshSource, meshAPITimeout)
		if err != nil {
			return nil, err
		}
		tools = append(tools, meshTools...)
	}

	for _, t := range tools {
		slog.Info("Loaded tool", "tool", t.Name())
	}

	// Create the main agent
	agentCfg := llmagent.Config{
		Name:        "chat_agent",
		Description: "A smart assistant handling chat communications.",
		Model:       geminiModel,
		Instruction: systemInstruction + "\n" + extraContext,
		Tools:       tools,
	}

	chatAgent, err := llmagent.New(agentCfg)
	if err != nil {
		return nil, err
	}

	// Create the runner bounded to an in-memory session service
	runnerCfg := runner.Config{
		AppName:           "chatty",
		Agent:             chatAgent,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	}

	r, err := runner.New(runnerCfg)
	if err != nil {
		return nil, err
	}

	return r, nil
}
