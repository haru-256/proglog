// Package server implements the gRPC server for the distributed log service.
package server

import (
	"context"

	api "github.com/haru-256/proglog/api/v1"
	"google.golang.org/grpc"
)

// CommitLog defines the interface for interacting with the commit log.
// It provides methods for appending records and reading records by offset.
type CommitLog interface {
	// Append appends a record to the log and returns the absolute offset of the record.
	Append(record *api.Record) (uint64, error)
	// Read retrieves a record from the log at the specified offset.
	Read(offset uint64) (*api.Record, error)
}

// Config holds the configuration for the gRPC server.
// It includes a CommitLog instance for log operations.
type Config struct {
	CommitLog CommitLog // The underlying commit log implementation
}

// NewGRPCServer creates and configures a new gRPC server.
// It initializes the server, registers the log service, and returns the server instance.
// The server uses the provided configuration and optional gRPC server options (e.g., TLS settings).
func NewGRPCServer(config *Config, grpcOpts ...grpc.ServerOption) (*grpc.Server, error) {
	gsrv := grpc.NewServer(grpcOpts...)
	srv, err := newgrpcServer(config)
	if err != nil {
		return nil, err
	}
	api.RegisterLogServer(gsrv, srv) // Register the LogServer implementation with the gRPC server
	return gsrv, nil
}

var _ api.LogServer = (*grpcServer)(nil)

// grpcServer implements the api.LogServer interface.
// It handles gRPC requests for producing and consuming log records.
type grpcServer struct {
	api.UnimplementedLogServer // Embed UnimplementedLogServer to ensure forward compatibility.
	*Config                    // The server configuration.
}

// newgrpcServer creates a new instance of grpcServer.
// It initializes the server with the provided configuration.
func newgrpcServer(config *Config) (srv *grpcServer, err error) {
	srv = &grpcServer{
		Config: config,
	}
	return srv, nil
}

// Produce handles a single produce request.
// It appends the provided record to the commit log and returns the offset.
func (s *grpcServer) Produce(ctx context.Context, req *api.ProduceRequest) (*api.ProduceResponse, error) {
	offset, err := s.CommitLog.Append(req.Record)
	if err != nil {
		return nil, err
	}
	return &api.ProduceResponse{Offset: offset}, nil
}

// Consume handles a single consume request.
// It reads a record from the commit log at the specified offset.
func (s *grpcServer) Consume(ctx context.Context, req *api.ConsumeRequest) (*api.ConsumeResponse, error) {
	record, err := s.CommitLog.Read(req.Offset)
	if err != nil {
		return nil, err
	}
	return &api.ConsumeResponse{Record: record}, nil
}

// ProduceStream handles a stream of produce requests.
// It continuously receives records from the client stream and appends them to the log.
func (s *grpcServer) ProduceStream(stream api.Log_ProduceStreamServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		res, err := s.Produce(stream.Context(), req)
		if err != nil {
			return err
		}
		if err := stream.Send(res); err != nil {
			return err
		}
	}
}

// ConsumeStream handles a stream of consume requests.
// It continuously sends records to the client stream, starting from the requested offset.
// If the requested offset is out of range, it waits for new records to become available.
func (s *grpcServer) ConsumeStream(req *api.ConsumeRequest, stream api.Log_ConsumeStreamServer) error {
	for {
		select {
		case <-stream.Context().Done():
			return nil
		default:
			res, err := s.Consume(stream.Context(), req)
			switch err.(type) {
			case nil:
			case api.ErrOffsetOutOfRange:
				// TODO: データが追加されるまで待機するか、Sleepを入れて再試行する
				// If the offset is out of range, continue to wait for new data.
				// A more sophisticated implementation might use a watcher or a conditional variable.
				continue
			default:
				return err
			}
			if err = stream.Send(res); err != nil {
				return err
			}
			req.Offset++
		}
	}
}
