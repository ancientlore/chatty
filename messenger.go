package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/genai"
)

type request struct {
	Msg      string
	Metadata map[string]string
	RespChan chan response
}

type response struct {
	Text string
	Err  error
}

func router(ctx context.Context, client *genai.Client, model, systemInstruction string) (chan<- request, error) {
	chats := make(map[string]chan<- request)
	input := make(chan request)

	defer func() {
		for name, ch := range chats {
			slog.Info("closing chat channel", "chat", name)
			close(ch)
		}
	}()

	go func() {
		for msg := range input {
			name := msg.Metadata["channel"]
			if name == "DM" || name == "" {
				name = msg.Metadata["node_id"]
			}
			if name == "" {
				msg.RespChan <- response{Err: fmt.Errorf("no channel or node_id found")}
				continue
			}

			ch, ok := chats[name]
			if !ok {
				var err error
				slog.Info("creating new chat", "chat", name)
				ch, err = messenger(ctx, client, model, systemInstruction)
				if err != nil {
					msg.RespChan <- response{Err: err}
					continue
				}
				chats[name] = ch
			}
			ch <- msg
		}
	}()

	return input, nil
}

func messenger(ctx context.Context, client *genai.Client, model, systemInstruction string) (chan<- request, error) {
	config := &genai.GenerateContentConfig{}
	if systemInstruction != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: systemInstruction}},
		}
	}

	chat, err := client.Chats.Create(ctx, model, config, nil)
	if err != nil {
		return nil, err
	}

	input := make(chan request)

	go func() {
		for msg := range input {
			// If history is getting long, restart the chat
			history := chat.History(true)
			// slog.Info("history", "historyLen", len(history), "history", fmt.Sprintf("%+v", history))
			if len(history) > 20 {
				chat, err = client.Chats.Create(ctx, model, config, history[10:])
				if err != nil {
					msg.RespChan <- response{Err: err}
					continue
				}
			}

			// Process message
			parts := []genai.Part{}
			if len(msg.Metadata) > 0 {
				var meta string
				for k, v := range msg.Metadata {
					meta += fmt.Sprintf("%s: %s\n", strings.ToUpper(k), v)
				}
				parts = append(parts, genai.Part{Text: meta})
			}
			parts = append(parts, genai.Part{Text: msg.Msg})

			resp, err := chat.SendMessage(ctx, parts...)
			if err != nil {
				msg.RespChan <- response{Err: err}
				continue
			}
			if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
				msg.RespChan <- response{Text: ""}
				continue
			}
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.Text != "" {
					msg.RespChan <- response{Text: part.Text}
					break
				}
			}
		}
	}()

	return input, nil
}
