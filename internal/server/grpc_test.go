package server

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	rpkirtripb "github.com/mellowdrifter/rpkirtr2/api/v1"
	"github.com/mellowdrifter/rpkirtr2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestGRPCStats(t *testing.T) {
	cfg := &config.Config{
		ListenAddr: "127.0.0.1:0",
		GRPCAddr:   "127.0.0.1:0",
		LogLevel:   "error",
		RPKIURLs:   []string{"http://example.com/roas.json"},
	}
	srv := New(cfg, zaptest.NewLogger(t).Sugar())

	// Manually set some ROAs so we have stats
	srv.LoadROAs([]ROA{
		{Prefix: netip.MustParsePrefix("1.1.1.0/24"), ASN: 1, MaxMask: 24},
	})

	// Start gRPC server manually for testing
	l, err := net.Listen("tcp", cfg.GRPCAddr)
	require.NoError(t, err)
	addr := l.Addr().String()

	gs := grpc.NewServer()
	rpkirtripb.RegisterRPKIRTRServiceServer(gs, &grpcServer{srv: srv})

	go func() {
		_ = gs.Serve(l)
	}()
	defer gs.Stop()

	// Connect to gRPC server
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := rpkirtripb.NewRPKIRTRServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetStats(ctx, &rpkirtripb.GetStatsRequest{})
	require.NoError(t, err)

	assert.Equal(t, uint32(1), resp.RoaCount)
	assert.Equal(t, uint32(0), resp.ClientCount)
	assert.Equal(t, srv.cache.serial, resp.Serial)
}
