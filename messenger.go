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

	const extraContext = "Use the following context:\nNode ID: {node_id?}\nShort Name: {short_name?}\nLong Name: {long_name?}\nHops: {hops?}\nSNR: {snr?}\nRSSI: {rssi?}\nNode Count: {node_count?}"

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
