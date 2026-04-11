package main

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/genai"
)

func initTracer() (*trace.TracerProvider, error) {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

func main() {
	var (
		addr    string
		token   string
		system  string
		prefix  string
		otelOut bool
	)

	flag.StringVar(&addr, "addr", ":8080", "TCP host:port to listen on")
	flag.StringVar(&system, "system", "system.txt", "Path to system instructions file")
	flag.StringVar(&prefix, "prefix", "", "Prefix to include in response")
	flag.BoolVar(&otelOut, "otel", false, "Output OpenTelemetry spans to stdout")
	flag.Parse()

	if otelOut {
		tp, err := initTracer()
		if err != nil {
			slog.Error("failed to initialize tracer", "error", err)
			os.Exit(1)
		}
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				slog.Error("error shutting down tracer provider", "error", err)
			}
		}()
	}

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

	run, err := buildRunner(context.Background(), token, "gemini-2.5-flash-lite", systemInstruction)
	if err != nil {
		slog.Error("failed to create runner", "error", err)
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

		metadata := make(map[string]any)
		for _, key := range []string{"channel", "node_id", "short_name", "long_name", "hops", "snr", "rssi", "node_count"} {
			value := r.URL.Query().Get(key)
			if value != "" {
				metadata[key] = value
			}
		}

		sessionID := r.URL.Query().Get("channel")
		if sessionID == "DM" || sessionID == "" {
			sessionID = r.URL.Query().Get("node_id")
		}
		if sessionID == "" {
			sessionID = "default"
		}
		// slog.Info("creating new chat", "session_id", sessionID)

		ctx := r.Context()
		userContent := &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{{Text: msg}},
		}

		var opts []runner.RunOption
		if len(metadata) > 0 {
			opts = append(opts, runner.WithStateDelta(metadata))
		}

		var respText string
		events := run.Run(ctx, sessionID, sessionID, userContent, agent.RunConfig{}, opts...)
		for event, err := range events {
			if err != nil {
				slog.Error("failed to get response from AI", "error", err)
				http.Error(w, "failed to get response from AI", http.StatusInternalServerError)
				return
			}
			if event.Content != nil {
				for _, part := range event.Content.Parts {
					if part.Text != "" {
						respText += part.Text
					}
				}
			}
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if len(prefix) > 0 {
			w.Write([]byte(prefix))
		}
		w.Write([]byte(respText))
		if len([]byte(respText)) > 200 {
			slog.Warn("response too long", "length", len([]byte(respText)), "response", respText)
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

type loggingResponseWriter struct {
	http.ResponseWriter
	result bytes.Buffer
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	w.result.Write(b)
	return w.ResponseWriter.Write(b)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var wr loggingResponseWriter
		wr.ResponseWriter = w

		// Wrap the ResponseWriter to capture the status code
		wrapped := &wrappedWriter{
			ResponseWriter: &wr,
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
			"response", wr.result.String(),
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
