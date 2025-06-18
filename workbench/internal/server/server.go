// Package server implements the gRPC server for the distributed log service.
package server

import (
	"context"

	api "github.com/haru-256/proglog/api/v1"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Authorizer defines the interface for authorization operations.
// It provides methods to check permissions for subjects performing actions on objects.
type Authorizer interface {
	// Authorize checks if the given subject is permitted to perform the specified action on the object.
	// Returns an error if authorization fails, or nil if access is granted.
	Authorize(subject, object, action string) error
}

// CommitLog defines the interface for interacting with the commit log.
// It provides methods for appending records and reading records by offset.
type CommitLog interface {
	// Append appends a record to the log and returns the absolute offset of the record.
	Append(record *api.Record) (uint64, error)
	// Read retrieves a record from the log at the specified offset.
	Read(offset uint64) (*api.Record, error)
}

// Config holds the configuration for the gRPC server.
// It includes dependencies required for server operations.
type Config struct {
	CommitLog  CommitLog  // The underlying commit log implementation for data persistence
	Authorizer Authorizer // The authorization component for access control
}

const (
	objectWildcard = "*"       // Wildcard object identifier for authorization
	produceAction  = "produce" // Action identifier for producing/writing to log
	consumeAction  = "consume" // Action identifier for consuming/reading from log
)

// NewGRPCServer creates and configures a new gRPC server.
// It initializes the server with authentication middleware, registers the log service,
// and returns the configured server instance.
//
// The server automatically includes:
//   - Authentication interceptors for both unary and streaming RPC calls
//   - Authorization checks for all operations
//   - TLS support when configured via grpcOpts
//
// Parameters:
//   - config: Server configuration including CommitLog and Authorizer
//   - grpcOpts: Optional gRPC server options (e.g., TLS credentials, timeouts)
//
// Returns the configured gRPC server ready to serve requests.
func NewGRPCServer(config *Config, grpcOpts ...grpc.ServerOption) (*grpc.Server, error) {
	grpcOpts = append(
		grpcOpts,
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				grpc_auth.StreamServerInterceptor(authenticate),
			)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			grpc_auth.UnaryServerInterceptor(authenticate),
		)),
	)
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
// It initializes the server with the provided configuration and validates dependencies.
func newgrpcServer(config *Config) (srv *grpcServer, err error) {
	srv = &grpcServer{
		Config: config,
	}
	return srv, nil
}

// Produce handles a single produce request.
// It validates authorization, appends the provided record to the commit log,
// and returns the assigned offset.
//
// The method performs the following steps:
//  1. Authorizes the client to perform produce operations
//  2. Appends the record to the commit log
//  3. Returns the assigned offset to the client
//
// Returns a gRPC error if authorization fails or if the log operation encounters an error.
func (s *grpcServer) Produce(ctx context.Context, req *api.ProduceRequest) (*api.ProduceResponse, error) {
	if err := s.Authorizer.Authorize(subject(ctx), objectWildcard, produceAction); err != nil {
		return nil, err
	}

	offset, err := s.CommitLog.Append(req.Record)
	if err != nil {
		return nil, err
	}
	return &api.ProduceResponse{Offset: offset}, nil
}

// Consume handles a single consume request.
// It validates authorization, reads a record from the commit log at the specified offset,
// and returns the record to the client.
//
// The method performs the following steps:
//  1. Authorizes the client to perform consume operations
//  2. Reads the record from the commit log at the requested offset
//  3. Returns the record to the client
//
// Returns a gRPC error if authorization fails or if the requested offset is invalid.
func (s *grpcServer) Consume(ctx context.Context, req *api.ConsumeRequest) (*api.ConsumeResponse, error) {
	if err := s.Authorizer.Authorize(subject(ctx), objectWildcard, consumeAction); err != nil {
		return nil, err
	}

	record, err := s.CommitLog.Read(req.Offset)
	if err != nil {
		return nil, err
	}
	return &api.ConsumeResponse{Record: record}, nil
}

// ProduceStream handles a stream of produce requests.
// It continuously receives records from the client stream, validates authorization
// for each record, appends them to the log, and sends back the assigned offsets.
//
// The stream remains open until the client closes it or an error occurs.
// Each record is processed individually with its own authorization check.
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
//
// The stream automatically advances the offset after each successful read, allowing
// clients to consume a continuous stream of records. The stream respects context
// cancellation and will terminate gracefully when the client disconnects.
//
// Note: The current implementation uses a simple polling approach when waiting for
// new records. In production, this could be optimized with a more sophisticated
// notification mechanism.
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

// authenticate is a gRPC authentication interceptor that extracts client identity from TLS certificates.
// It validates the client's TLS certificate and extracts the Common Name (CN) from the certificate
// as the subject identifier for authorization purposes.
//
// The function performs the following steps:
//  1. Extracts peer information from the gRPC context
//  2. Validates TLS authentication info is present
//  3. Verifies the client certificate chain
//  4. Extracts the subject (CN) from the verified certificate
//  5. Stores the subject in the context for use by authorization logic
//
// Returns a context with the authenticated subject, or an error if authentication fails.
// If no client certificate is provided, an empty subject is used (for anonymous access).
func authenticate(ctx context.Context) (context.Context, error) {
	// gRPCサーバーがクライアントからRPCリクエストを受け取ると、gRPCフレームワークは自動的にそのクライアントの接続情報を context.Context に格納する
	// リクエストを送信してきたクライアント（Peer）の接続情報を取得
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return ctx, status.New(codes.Unknown, "couldn't get peer info").Err()
	}
	if peer.AuthInfo == nil {
		return ctx, status.New(codes.Unauthenticated, "no authentication info found").Err()
	}
	tlsInfo := peer.AuthInfo.(credentials.TLSInfo)
	// VerifiedChainsが空 = クライアント証明書が検証されていない
	// VerifiedChains[0] が1つ目の検証済みチェーン
	// VerifiedChains[0][0] がそのチェーンのリーフ証明書（クライアント自身の証明書）
	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return ctx, status.New(codes.Unauthenticated, "no verified chains found").Err()
	}
	// サーバーによる検証が済んだVerifiedChainsからアイデンティティを取得
	subject := tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
	ctx = context.WithValue(ctx, subjectContextKey{}, subject)
	return ctx, nil
}

// subject extracts the authenticated subject identifier from the request context.
// Returns the subject string that was previously stored by the authenticate interceptor.
// The subject is typically the Common Name (CN) from the client's TLS certificate.
func subject(ctx context.Context) string {
	return ctx.Value(subjectContextKey{}).(string)
}

// subjectContextKey is a private type used as a key for storing the authenticated subject in context.
// Using a private type prevents key collisions with other context values.
type subjectContextKey struct{}
