// Package agent provides a distributed log agent that combines multiple components
// to create a complete distributed logging system with replication and service discovery.
package agent

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"

	api "github.com/haru-256/proglog/api/v1"

	"github.com/haru-256/proglog/internal/auth"
	"github.com/haru-256/proglog/internal/discovery"
	"github.com/haru-256/proglog/internal/log"
	"github.com/haru-256/proglog/internal/server"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Agent represents a complete distributed log node that orchestrates multiple components.
// It manages the commit log, gRPC server, cluster membership, and replication functionality.
// The Agent provides a high-level interface for running a distributed log service node.
type Agent struct {
	Config // Embedded configuration

	log        *log.Log              // Local commit log for storing records
	server     *grpc.Server          // gRPC server for handling client requests
	membership *discovery.Membership // Cluster membership management via Serf
	replicator *log.Replicator       // Handles replication to/from other nodes

	shutdown     bool          // Flag indicating if shutdown has been initiated
	shutdowns    chan struct{} // Channel for coordinating shutdown
	shutdownLock sync.Mutex    // Mutex for thread-safe shutdown operations
}

// Config holds all configuration parameters for an Agent instance.
// It includes network settings, security configurations, and cluster parameters.
type Config struct {
	ServerTLSConfig *tls.Config // TLS configuration for client-server communication
	PeerTlsConfig   *tls.Config // TLS configuration for peer-to-peer communication for replication
	DataDir         string      // Directory path for storing log data
	BindAddr        string      // Network address for binding services of discovery and replication (IP:port)
	RPCPort         int         // Port number for gRPC server
	NodeName        string      // Unique identifier for this node in the cluster
	StartJoinAddrs  []string    // Addresses of existing cluster members to join on startup
	ACLModelFile    string      // Path to access control model configuration file
	ACLPolicyFile   string      // Path to access control policy configuration file
}

// RPCAddr constructs the RPC address by combining the host from BindAddr with RPCPort.
// It extracts the host portion from BindAddr and appends the configured RPC port.
// Returns the complete RPC address string or an error if BindAddr parsing fails.
func (c Config) RPCAddr() (string, error) {
	host, _, err := net.SplitHostPort(c.BindAddr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", host, c.RPCPort), nil
}

// New creates and initializes a new Agent with the provided configuration.
// It sets up all required components in the correct order: logger, log, server, and membership.
// Returns a fully configured Agent ready to serve requests, or an error if setup fails.
func New(config Config) (*Agent, error) {
	a := &Agent{
		Config:    config,
		shutdowns: make(chan struct{}),
	}
	// Setup components in dependency order
	setup := []func() error{
		a.setupLogger,
		a.setupLog,
		a.setupServer,
		a.setupMembership,
	}
	for _, fn := range setup {
		if err := fn(); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// setupLogger initializes the global zap logger for development use.
// It configures a development logger with console output and replaces the global logger instance.
func (a *Agent) setupLogger() error {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return err
	}
	zap.ReplaceGlobals(logger)
	return nil
}

// setupLog initializes the local commit log for storing records.
// It creates a new log instance using the configured data directory.
func (a *Agent) setupLog() error {
	var err error
	a.log, err = log.NewLog(
		a.Config.DataDir,
		log.Config{},
	)
	return err
}

// setupServer configures and starts the gRPC server for handling client requests.
// It sets up authorization, TLS credentials, and starts serving in a background goroutine.
func (a *Agent) setupServer() error {
	// Initialize access control with model and policy files
	authorizer, _ := auth.New(
		a.Config.ACLModelFile,
		a.Config.ACLPolicyFile,
	)
	serverConfig := &server.Config{
		CommitLog:  a.log,
		Authorizer: authorizer,
	}

	// Configure TLS credentials if specified
	var opts []grpc.ServerOption
	if a.Config.ServerTLSConfig != nil {
		creds := credentials.NewTLS(a.Config.ServerTLSConfig)
		opts = append(opts, grpc.Creds(creds))
	}

	// Create and configure the gRPC server
	var err error
	a.server, err = server.NewGRPCServer(serverConfig, opts...)
	if err != nil {
		return err
	}

	// Start listening on the configured RPC address
	rpcAddr, err := a.Config.RPCAddr()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", rpcAddr)
	if err != nil {
		return err
	}

	// Start serving requests in a background goroutine
	go func() {
		if err := a.server.Serve(ln); err != nil {
			_ = a.Shutdown()
		}
	}()
	return nil
}

// setupMembership initializes cluster membership and replication functionality.
// It creates a replicator for handling data replication and joins the cluster using Serf.
func (a *Agent) setupMembership() error {
	rpcAddr, err := a.Config.RPCAddr()
	if err != nil {
		return err
	}

	// Configure TLS for peer-to-peer communication
	var opts []grpc.DialOption
	if a.Config.PeerTlsConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(
			credentials.NewTLS(a.Config.PeerTlsConfig),
		))
	}

	// Create a connection to the local server for replication
	conn, err := grpc.NewClient(rpcAddr, opts...)
	if err != nil {
		return err
	}
	client := api.NewLogClient(conn)

	// Initialize the replicator with dial options and local server client
	a.replicator = &log.Replicator{
		DialOptions: opts,
		LocalServer: client,
	}

	// Join the cluster using Serf for membership management
	a.membership, err = discovery.New(a.replicator, discovery.Config{
		NodeName: a.Config.NodeName,
		BindAddr: a.Config.BindAddr,
		Tags: map[string]string{
			"rpc_addr": rpcAddr,
		},
		StartJoinAddrs: a.Config.StartJoinAddrs,
	})
	return err
}

// Shutdown gracefully shuts down all Agent components in the correct order.
// It ensures thread-safe shutdown and prevents multiple shutdown attempts.
// Components are shut down in reverse dependency order: membership, log, server, replicator.
func (a *Agent) Shutdown() error {
	a.shutdownLock.Lock()
	defer a.shutdownLock.Unlock()
	if a.shutdown {
		return nil
	}
	a.shutdown = true
	close(a.shutdowns)

	// Shutdown components in reverse dependency order
	shutdown := []func() error{
		a.membership.Leave,
		a.log.Close,
		func() error {
			a.server.GracefulStop()
			return nil
		},
		a.replicator.Close,
	}
	for _, fn := range shutdown {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}
