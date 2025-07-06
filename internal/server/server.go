package server

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/config"
	"go.uber.org/zap"
)

type Server struct {
	cfg          *config.Config
	logger       *zap.SugaredLogger
	listener     net.Listener
	wg           sync.WaitGroup
	mu           sync.Mutex
	shuttingDown bool
	clients      map[string]*Client // map of client ID to client struct
	mutex        *sync.RWMutex
	diffs        diffs
	urls         []string
}

type roa struct {
	Prefix  netip.Prefix
	MaxMask uint8
	ASN     uint32
}

type diffs struct {
	old    uint32
	new    uint32
	delRoa []roa
	addRoa []roa
	diff   bool
}

const (
	refreshROA = 6 * time.Minute
)

// New creates a new Server instance
func New(cfg *config.Config, logger *zap.SugaredLogger) *Server {
	return &Server{
		cfg:     cfg,
		logger:  logger,
		clients: make(map[string]*Client),
	}
}

// Start begins listening and accepting client connections
func (s *Server) Start() error {
	l, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = l
	s.logger.Infof("Listening on %s", s.cfg.ListenAddr)

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
