package opamp

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/conduit-obs/conduit/internal/eventbus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	pb "github.com/conduit-obs/conduit/internal/opamp/proto"
)

// ServerConfig holds OpAMP server configuration.
type ServerConfig struct {
	ListenAddr string
	CertFile   string
	KeyFile    string
	CAFile     string // CA for verifying client certs (mTLS)
}

// Server is the OpAMP gRPC server with mTLS support.
type Server struct {
	pb.UnimplementedOpAMPServiceServer
	config     ServerConfig
	grpcServer *grpc.Server
	tracker    *HeartbeatTracker
	eventBus   *eventbus.Bus
	logger     *slog.Logger

	mu          sync.RWMutex
	connections map[string]*agentConnection
}

type agentConnection struct {
	instanceID string
	tenantID   string
	stream     pb.OpAMPService_ConnectServer
}

// NewServer creates a new OpAMP server.
func NewServer(cfg ServerConfig, tracker *HeartbeatTracker, bus *eventbus.Bus, logger *slog.Logger) *Server {
	return &Server{
		config:      cfg,
		tracker:     tracker,
		eventBus:    bus,
		logger:      logger,
		connections: make(map[string]*agentConnection),
	}
}

// Start begins listening for OpAMP connections with mTLS.
func (s *Server) Start() (net.Listener, error) {
	tlsConfig, err := s.buildTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("building TLS config: %w", err)
	}

	creds := credentials.NewTLS(tlsConfig)
	s.grpcServer = grpc.NewServer(grpc.Creds(creds))
	pb.RegisterOpAMPServiceServer(s.grpcServer, s)

	lis, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", s.config.ListenAddr, err)
	}

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			s.logger.Error("gRPC server error", "error", err)
		}
	}()

	s.logger.Info("OpAMP server started", "addr", lis.Addr().String())
	return lis, nil
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}

// buildTLSConfig creates a TLS configuration requiring client certificates.
func (s *Server) buildTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading server cert: %w", err)
	}

	caCertPool, err := loadCACertPool(s.config.CAFile)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func loadCACertPool(caFile string) (*x509.CertPool, error) {
	if caFile == "" {
		return nil, errors.New("CA file is required for mTLS")
	}
	// We use x509.SystemCertPool as base and add our CA
	pool := x509.NewCertPool()
	caCert, err := readFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert: %w", err)
	}
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse CA certificate")
	}
	return pool, nil
}

// PushConfig sends a config to a specific connected agent. Returns true if the agent was connected and config was sent.
func (s *Server) PushConfig(instanceID, configYAML string) bool {
	s.mu.RLock()
	conn, ok := s.connections[instanceID]
	s.mu.RUnlock()
	if !ok {
		return false
	}

	err := conn.stream.Send(&pb.ServerToAgent{
		InstanceUid:  instanceID,
		RemoteConfig: configYAML,
	})
	if err != nil {
		s.logger.Error("failed to push config", "instance_id", instanceID, "error", err)
		return false
	}

	s.tracker.SetEffectiveConfig(instanceID, configYAML)
	s.logger.Info("config pushed to agent", "instance_id", instanceID)
	return true
}

// PushConfigToTenantAgents sends config to all connected agents for a tenant whose instance IDs are in the given list.
// Returns the number of agents that received the config.
func (s *Server) PushConfigToTenantAgents(tenantID, configYAML string, targetInstanceIDs []string) int {
	targetSet := make(map[string]bool, len(targetInstanceIDs))
	for _, id := range targetInstanceIDs {
		targetSet[id] = true
	}

	s.mu.RLock()
	var targets []*agentConnection
	for _, conn := range s.connections {
		if conn.tenantID == tenantID && targetSet[conn.instanceID] {
			targets = append(targets, conn)
		}
	}
	s.mu.RUnlock()

	pushed := 0
	for _, conn := range targets {
		err := conn.stream.Send(&pb.ServerToAgent{
			InstanceUid:  conn.instanceID,
			RemoteConfig: configYAML,
		})
		if err != nil {
			s.logger.Error("failed to push config", "instance_id", conn.instanceID, "error", err)
			continue
		}
		s.tracker.SetEffectiveConfig(conn.instanceID, configYAML)
		pushed++
	}
	return pushed
}

// GetConnectedAgentIDsByTenant returns instance IDs of all connected agents for a tenant.
func (s *Server) GetConnectedAgentIDsByTenant(tenantID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ids []string
	for _, conn := range s.connections {
		if conn.tenantID == tenantID {
			ids = append(ids, conn.instanceID)
		}
	}
	return ids
}

// Connect handles the bidirectional agent-to-server stream.
func (s *Server) Connect(stream pb.OpAMPService_ConnectServer) error {
	// Extract client identity from mTLS peer certificate
	p, ok := peer.FromContext(stream.Context())
	if !ok {
		return errors.New("no peer info")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return errors.New("no TLS info")
	}

	var tenantID, instanceID string
	if len(tlsInfo.State.PeerCertificates) > 0 {
		cert := tlsInfo.State.PeerCertificates[0]
		tenantID = cert.Subject.Organization[0]
		instanceID = cert.Subject.CommonName
	}

	s.logger.Info("agent connected", "instance_id", instanceID, "tenant_id", tenantID)
	s.tracker.RecordHeartbeat(instanceID, tenantID)

	s.mu.Lock()
	s.connections[instanceID] = &agentConnection{
		instanceID: instanceID,
		tenantID:   tenantID,
		stream:     stream,
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.connections, instanceID)
		s.mu.Unlock()
		s.tracker.RemoveAgent(instanceID)
	}()

	for {
		msg, err := stream.Recv()
		if err != nil {
			s.logger.Info("agent disconnected", "instance_id", instanceID, "error", err)
			return nil
		}

		s.tracker.RecordHeartbeat(instanceID, tenantID)

		if msg.EffectiveConfig != "" {
			s.tracker.SetReportedConfig(instanceID, msg.EffectiveConfig)
		}

		resp := &pb.ServerToAgent{
			InstanceUid: instanceID,
		}
		if err := stream.Send(resp); err != nil {
			return fmt.Errorf("sending response: %w", err)
		}
	}
}
