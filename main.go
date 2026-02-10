package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/genai"
)

func main() {
	var (
		addr   string
		token  string
		system string
	)

	flag.StringVar(&addr, "addr", ":8080", "TCP host:port to listen on")
	flag.StringVar(&system, "system", "system.txt", "Path to system instructions file")
	flag.Parse()

	token = os.Getenv("GEMINI_API_KEY")
	if token == "" {
		slog.Error("GEMINI_API_KEY is required")
		os.Exit(1)
	}

	var systemInstruction string
	if content, err := os.ReadFile(system); err == nil {
		systemInstruction = string(content)
		slog.Info("loaded system instructions", "path", system)
	} else {
		if os.IsNotExist(err) {
			slog.Warn("system instruction file not found, proceeding without it", "path", system)
		} else {
			slog.Error("failed to read system instruction file", "error", err)
			os.Exit(1)
		}
	}

	chatModel, err := NewChatModel(context.Background(), token, systemInstruction)
	if err != nil {
		slog.Error("failed to create chat model", "error", err)
		os.Exit(1)
	}

	// Create a new ServeMux
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		msg := r.URL.Query().Get("msg")
		if msg == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("msg query parameter is required"))
			return
		}

		resp, err := chatModel.SendMessage(r.Context(), msg)
		if err != nil {
			slog.Error("failed to send message", "error", err)
			http.Error(w, "failed to get response from AI", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(resp))
		if len([]byte(resp)) > 200 {
			slog.Warn("response too long", "length", len([]byte(resp)), "response", resp)
		}
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Wrap the mux with the logging middleware
	loggedMux := loggingMiddleware(mux)

	// Create the HTTP server
	srv := &http.Server{
		Addr:    addr,
		Handler: loggedMux,
	}

	// Channel to listen for errors coming from the listener.
	serverErrors := make(chan error, 1)

	// Start the server
	go func() {
		slog.Info("Starting server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive our signal.
	select {
	case err := <-serverErrors:
		slog.Error("server error", "error", err)
	case sig := <-shutdown:
		slog.Info("shutdown started", "signal", sig)

		// Give outstanding requests a deadline for completion.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Asking listener to shutdown and shed load.
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("graceful shutdown did not complete in time", "error", err)
			if err := srv.Close(); err != nil {
				slog.Error("could not stop http server", "error", err)
			}
		}
	}

	slog.Info("shutdown complete", "addr", addr)
}

type ChatModel struct {
	client *genai.Client
	chat   *genai.Chat
}

func NewChatModel(ctx context.Context, token, systemInstruction string) (*ChatModel, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  token,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	config := &genai.GenerateContentConfig{}
	if systemInstruction != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: systemInstruction}},
		}
	}

	chat, err := client.Chats.Create(ctx, "gemini-2.5-flash-lite", config, nil)
	if err != nil {
		return nil, err
	}
	return &ChatModel{client: client, chat: chat}, nil
}

func (m *ChatModel) SendMessage(ctx context.Context, msg string) (string, error) {
	resp, err := m.chat.SendMessage(ctx, genai.Part{Text: msg})
	if err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", nil
	}
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			return part.Text, nil
		}
	}
	return "", nil
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the ResponseWriter to capture the status code
		wrapped := &wrappedWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default to 200
		}

		next.ServeHTTP(wrapped, r)

		slog.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.Query(),
			"status", wrapped.statusCode,
			// "headers", r.Header,
			"duration", time.Since(start),
		)
	})
}

type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.statusCode = statusCode
}
