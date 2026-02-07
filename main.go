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

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func main() {
	var (
		addr   string
		token  string
		system string
	)

	flag.StringVar(&addr, "addr", ":8080", "TCP host:port to listen on")
	flag.StringVar(&token, "token", "", "Google AI Token")
	flag.StringVar(&system, "system", "system.txt", "Path to system instructions file")
	flag.Parse()

	if token == "" {
		slog.Error("token is required")
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
	defer chatModel.Close()

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
	client  *genai.Client
	session *genai.ChatSession
}

func NewChatModel(ctx context.Context, token, systemInstruction string) (*ChatModel, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(token))
	if err != nil {
		return nil, err
	}
	model := client.GenerativeModel("gemini-2.5-flash")
	if systemInstruction != "" {
		model.SystemInstruction = genai.NewUserContent(genai.Text(systemInstruction))
	}
	session := model.StartChat()
	return &ChatModel{client: client, session: session}, nil
}

func (m *ChatModel) SendMessage(ctx context.Context, msg string) (string, error) {
	resp, err := m.session.SendMessage(ctx, genai.Text(msg))
	if err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", nil
	}
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			return string(txt), nil
		}
	}
	return "", nil
}

func (m *ChatModel) Close() {
	m.client.Close()
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
