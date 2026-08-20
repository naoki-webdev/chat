package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"realtime-chat/backend/internal/ai"
)

type server struct {
	repository repository
	aiService  ai.Service
	hub        *hub
	upgrader   websocket.Upgrader
	aiMu       sync.Mutex
	aiInFlight map[string]int
	aiLastRun  map[string]time.Time
}

func newServer() *server {
	return newServerWithRepositoryAndAI(newMemoryRepository(), ai.NewMock())
}

func newServerWithRepository(repository repository) *server {
	return newServerWithRepositoryAndAI(repository, ai.NewFromEnv())
}

func newServerWithRepositoryAndAI(repository repository, service ai.Service) *server {
	return &server{
		repository: repository,
		aiService:  service,
		hub:        newHub(),
		aiInFlight: make(map[string]int),
		aiLastRun:  make(map[string]time.Time),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(request *http.Request) bool {
				return isAllowedOrigin(request.Header.Get("Origin"))
			},
		},
	}
}

func newProductionServer(ctx context.Context) (*server, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" && !isLocalEnvironment() {
		return nil, errors.New("DATABASE_URL must be set outside development and test environments")
	}
	if !isLocalEnvironment() {
		if err := ai.ValidateProductionConfig(); err != nil {
			return nil, err
		}
	}
	if databaseURL == "" {
		log.Printf("DATABASE_URL is not set; using in-memory store for local development")
		return newServer(), nil
	}
	repository, err := newPostgresRepository(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return newServerWithRepository(repository), nil
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/auth/register", s.handleRegister)
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/me", s.handleCurrentUser)
	mux.HandleFunc("/api/channels", s.handleChannels)
	mux.HandleFunc("/api/channels/", s.handleChannelRoutes)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/messages/", s.handleMessageRoutes)
	mux.HandleFunc("/ws", s.handleWebSocket)
	return withLogging(withCORS(mux))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	application, err := newProductionServer(ctx)
	if err != nil {
		log.Fatalf("could not initialize repository: %v", err)
	}
	defer application.repository.Close()
	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           application.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("Orbit Chat API listening on http://localhost:%s", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
