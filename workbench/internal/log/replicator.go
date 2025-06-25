package log

import (
	"context"
	"sync"

	api "github.com/haru-256/proglog/api/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Replicator handles data replication between distributed log nodes.
// It manages connections to remote servers and continuously replicates their logs
// to the local server, ensuring data consistency across the cluster.
//
// The Replicator implements the discovery.Handler interface, responding to
// cluster membership events by starting or stopping replication streams.
type Replicator struct {
	DialOptions []grpc.DialOption // gRPC dial options for connecting to remote servers
	LocalServer api.LogClient     // Client interface to the local log server

	logger *zap.Logger // Logger for replication events and errors

	mu      sync.Mutex               // Mutex for thread-safe operations
	servers map[string]chan struct{} // Map of server names to stop channels for replication control
	closed  bool                     // Flag indicating if the replicator has been shut down
	close   chan struct{}            // Channel for coordinating global shutdown
}

// Join adds a new server to the replication target list and starts replicating from it.
// This method is called when a new node joins the cluster via the discovery mechanism.
// It starts a background goroutine that continuously streams records from the remote server.
//
// Parameters:
//   - name: Unique identifier for the server in the cluster
//   - addr: Network address of the server to replicate from
//
// Returns an error if the replicator is closed, otherwise starts replication asynchronously.
func (r *Replicator) Join(name, addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()

	if r.closed {
		r.servers = make(map[string]chan struct{})
	}

	if _, ok := r.servers[name]; ok {
		// Already replicating from this server, skip
		return nil
	}
	r.servers[name] = make(chan struct{})

	// Start replication in a background goroutine
	go r.replicate(addr, r.servers[name])

	return nil
}

// replicate continuously streams records from a remote server and produces them locally.
// It establishes a gRPC connection, creates a consume stream, and forwards all received
// records to the local server. The function runs until the server leaves the cluster
// or the replicator is shut down.
func (r *Replicator) replicate(addr string, leave chan struct{}) {
	// Establish connection to the remote server
	cc, err := grpc.NewClient(addr, r.DialOptions...)
	if err != nil {
		r.logError(err, "failed to dial", addr)
		return
	}
	defer cc.Close()

	client := api.NewLogClient(cc)

	// Start consuming from offset 0 to get all records
	ctx := context.Background()
	stream, err := client.ConsumeStream(ctx, &api.ConsumeRequest{Offset: 0})
	if err != nil {
		r.logError(err, "failed to consume stream", addr)
		return
	}

	// Channel for buffering incoming records from the stream
	records := make(chan *api.Record)
	go func() {
		for {
			recv, err := stream.Recv()
			if err != nil {
				r.logError(err, "failed to receive record", addr)
				return
			}
			records <- recv.Record
		}
	}()

	// Main replication loop: forward received records to local server
	for {
		select {
		case <-r.close:
			return // Global shutdown
		case <-leave:
			return // Server left the cluster
		case record := <-records:
			// Replicate the record to the local server
			_, err := r.LocalServer.Produce(ctx,
				&api.ProduceRequest{
					Record: record,
				},
			)
			if err != nil {
				r.logError(err, "failed to produce", addr)
				return
			}
		}
	}
}

// Leave stops replication from the specified server and removes it from the active list.
// This method is called when a node leaves the cluster via the discovery mechanism.
// It closes the stop channel for the server, causing the replication goroutine to exit.
func (r *Replicator) Leave(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()

	if _, ok := r.servers[name]; !ok {
		return nil // Server not found, nothing to do
	}

	// Signal the replication goroutine to stop
	close(r.servers[name])
	delete(r.servers, name)
	return nil
}

// init lazily initializes the replicator's internal state.
// It ensures that logger, servers map, and close channel are properly initialized.
func (r *Replicator) init() {
	if r.logger == nil {
		r.logger = zap.L().Named("replicator")
	}
	if r.servers == nil {
		r.servers = make(map[string]chan struct{})
	}
	if r.close == nil {
		r.close = make(chan struct{})
	}
}

// Close shuts down the replicator and stops all active replication streams.
// It signals all replication goroutines to stop and marks the replicator as closed.
// This method is idempotent and can be called multiple times safely.
func (r *Replicator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()

	if r.closed {
		return nil // Already closed
	}
	r.closed = true
	close(r.close) // Signal all replication goroutines to stop
	return nil
}

// logError logs replication errors with contextual information.
// It includes the server address and error details for debugging purposes.
func (r *Replicator) logError(err error, msg, addr string) {
	r.logger.Error(
		msg,
		zap.String("addr", addr),
		zap.Error(err),
	)
}
