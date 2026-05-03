package main

import (
	"context"

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
	"google.golang.org/genai"
)

func buildRunner(ctx context.Context, token, modelName, systemInstruction string) (*runner.Runner, error) {
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

	const extraContext = `Here is the real-time radio network telemetry for the user you are chatting with:
- Node ID: {node_id?} (The unique identifier of their device on the mesh network)
- Short Name: {short_name?} (The 4-character abbreviation of their device name)
- Long Name: {long_name?} (The full name of their device)
- Hops: {hops?} (The number of times the message was relayed to reach us; 0 means a direct connection)
- SNR: {snr?} (Signal-to-Noise Ratio in dB; higher is better, typically ranges from -20 to +10)
- RSSI: {rssi?} (Received Signal Strength Indicator in dBm; closer to 0 is better, e.g., -40 is excellent, -120 is very poor)
Here is some information about the network that is visible to you:
- Node Count: {node_count?} (Total number of active nodes currently seen by your device on the mesh network)
- Direct Count: {direct_count?} (Number of nodes directly connected/visible to your device without relays)
`

	// Create the main agent
	agentCfg := llmagent.Config{
		Name:        "chat_agent",
		Description: "A smart assistant handling chat communications.",
		Model:       geminiModel,
		Instruction: systemInstruction + "\n" + extraContext,
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
		},
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
