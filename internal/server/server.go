package server

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/config"
	"go.uber.org/zap"
)

type Server struct {
	// large fields first
	listener net.Listener
	logger   *zap.SugaredLogger
	cfg      *config.Config

	clients map[string]*Client
	urls    []string
	roas    []roa
	diffs   diffs

	// sync types next
	wg sync.WaitGroup
	mu *sync.RWMutex

	// smaller fields last
	serial       uint32
	session      uint16
	shuttingDown bool
}

const (
	refreshROA = 6 * time.Minute
)

// New creates a new Server instance
func New(cfg *config.Config, logger *zap.SugaredLogger) *Server {
	return &Server{
		logger:  logger,
		cfg:     cfg,
		clients: make(map[string]*Client),
		urls:    cfg.RPKIURLs,
		roas:    make([]roa, 0, 700000), // initial capacity for performance
		diffs:   diffs{},
		wg:      sync.WaitGroup{},
		mu:      &sync.RWMutex{},
	}
}

// Start begins listening and accepting client connections
func (s *Server) Start() error {
	ctx := context.Background()

	// Load initial ROAs before listening
	roas, err := s.loadROAs(ctx)
	if err != nil {
		return fmt.Errorf("failed to load initial ROAs: %w", err)
	}
	s.mu.Lock()
	s.roas = roas
	s.mu.Unlock()
	s.logger.Infof("Loaded %d initial ROAs", len(s.roas))

	l, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.cfg.ListenAddr, err)
	}
	s.listener = l
	s.logger.Infof("Listening on %s", s.cfg.ListenAddr)

	// Start background update ticker
	go s.periodicROAUpdater(ctx)

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.shuttingDown {
				return nil // graceful exit
			}
			s.logger.Errorf("accept error: %v", err)
			continue
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a new client
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	client := NewClient(conn, s.logger) // you'll define this in client_handler.go
	id := client.ID()

	s.mu.Lock()
	s.clients[id] = client
	s.mu.Unlock()

	s.logger.Infof("Client connected: %s", id)

	if err := client.Handle(); err != nil {
		s.logger.Warnf("Client %s error: %v", id, err)
	}

	s.mu.Lock()
	delete(s.clients, id)
	s.mu.Unlock()

	s.logger.Infof("Client disconnected: %s", id)
}

// Stop shuts down the server gracefully
func (s *Server) Stop(timeout time.Duration) error {
	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()

	s.logger.Info("Shutting down listener...")
	if s.listener != nil {
		s.listener.Close()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("All connections closed cleanly")
		return nil
	case <-time.After(timeout):
		s.logger.Warn("Shutdown timed out; some clients may still be active")
		return fmt.Errorf("timeout waiting for shutdown")
	}
}
