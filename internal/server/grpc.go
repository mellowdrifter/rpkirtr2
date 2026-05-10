package server

import (
	"context"

	rpkirtripb "github.com/mellowdrifter/rpkirtr2/api/v1"
)

type grpcServer struct {
	rpkirtripb.UnimplementedRPKIRTRServiceServer
	srv *Server
}

func (g *grpcServer) GetStats(ctx context.Context, req *rpkirtripb.GetStatsRequest) (*rpkirtripb.GetStatsResponse, error) {
	state := g.srv.cache.getState()

	g.srv.clientsMu.RLock()
	clientCount := uint32(len(g.srv.clients))
	g.srv.clientsMu.RUnlock()

	g.srv.upstreamsMu.RLock()
	upstreams := make([]*rpkirtripb.UpstreamStatus, 0, len(g.srv.upstreams))
	for url, stats := range g.srv.upstreams {
		upstreams = append(upstreams, &rpkirtripb.UpstreamStatus{
			Url:              url,
			LastFetchSuccess: stats.LastFetchSuccess,
			LastFetchTime:    stats.LastFetchTime.Unix(),
			ErrorMessage:     stats.ErrorMessage,
		})
	}
	g.srv.upstreamsMu.RUnlock()

	return &rpkirtripb.GetStatsResponse{
		RoaCount:    uint32(len(state.roas)),
		ClientCount: clientCount,
		Serial:      state.serial,
		LastUpdate:  state.lastUpdate.Unix(),
		Upstreams:   upstreams,
	}, nil
}
