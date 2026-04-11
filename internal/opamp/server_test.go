package opamp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/eventbus"
	pb "github.com/conduit-obs/conduit/internal/opamp/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// generateTestCerts creates a CA, server cert, and client cert for testing mTLS.
func generateTestCerts(t *testing.T) (caFile, serverCertFile, serverKeyFile, clientCertFile, clientKeyFile string) {
	t.Helper()
	dir := t.TempDir()

	// Generate CA key and cert
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Conduit Test CA"},
			CommonName:   "Conduit Test CA",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		t.Fatal(err)
	}

	caFile = filepath.Join(dir, "ca.pem")
	writePEM(t, caFile, "CERTIFICATE", caCertDER)

	// Generate server cert
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:    []string{"localhost"},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	serverCertFile = filepath.Join(dir, "server-cert.pem")
	serverKeyFile = filepath.Join(dir, "server-key.pem")
	writePEM(t, serverCertFile, "CERTIFICATE", serverCertDER)
	writeECKeyPEM(t, serverKeyFile, serverKey)

	// Generate client cert (agent cert with tenant in Org)
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization: []string{"tenant-test-123"},
			CommonName:   "agent-instance-001",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	clientCertFile = filepath.Join(dir, "client-cert.pem")
	clientKeyFile = filepath.Join(dir, "client-key.pem")
	writePEM(t, clientCertFile, "CERTIFICATE", clientCertDER)
	writeECKeyPEM(t, clientKeyFile, clientKey)

	return
}

func writePEM(t *testing.T, path, blockType string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pem.Encode(f, &pem.Block{Type: blockType, Bytes: data})
}

func writeECKeyPEM(t *testing.T, path string, key *ecdsa.PrivateKey) {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, path, "EC PRIVATE KEY", der)
}

func TestOpAMPServer_mTLSConnection(t *testing.T) {
	caFile, serverCert, serverKey, clientCert, clientKey := generateTestCerts(t)

	bus := eventbus.New()
	tracker := NewHeartbeatTracker(30*time.Second, bus)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	server := NewServer(ServerConfig{
		ListenAddr: "127.0.0.1:0",
		CertFile:   serverCert,
		KeyFile:    serverKey,
		CAFile:     caFile,
	}, tracker, bus, logger)

	lis, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	// Connect with client mTLS cert
	clientTLSCert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		t.Fatal(err)
	}

	caCertPEM, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCertPEM)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientTLSCert},
		RootCAs:      caCertPool,
		ServerName:   "localhost",
	}

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := pb.NewOpAMPServiceClient(conn)
	stream, err := client.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Send a message from the agent
	err = stream.Send(&pb.AgentToServer{
		InstanceUid:     "agent-instance-001",
		EffectiveConfig: "receivers:\n  otlp:\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Receive response
	resp, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}

	if resp.InstanceUid != "agent-instance-001" {
		t.Errorf("expected instance_uid agent-instance-001, got %s", resp.InstanceUid)
	}

	// Verify heartbeat was recorded
	agent, ok := tracker.GetAgent("agent-instance-001")
	if !ok {
		t.Fatal("agent not found in heartbeat tracker")
	}
	if agent.TenantID != "tenant-test-123" {
		t.Errorf("expected tenant tenant-test-123, got %s", agent.TenantID)
	}
	if agent.Status != "connected" {
		t.Errorf("expected connected, got %s", agent.Status)
	}
	if agent.ReportedConfig != "receivers:\n  otlp:\n" {
		t.Error("reported config not recorded")
	}

	stream.CloseSend()
}

func TestOpAMPServer_RejectsNoClientCert(t *testing.T) {
	caFile, serverCert, serverKey, _, _ := generateTestCerts(t)

	bus := eventbus.New()
	tracker := NewHeartbeatTracker(30*time.Second, bus)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := NewServer(ServerConfig{
		ListenAddr: "127.0.0.1:0",
		CertFile:   serverCert,
		KeyFile:    serverKey,
		CAFile:     caFile,
	}, tracker, bus, logger)

	lis, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	// Try to connect WITHOUT a client certificate
	caCertPEM, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCertPEM)

	tlsConfig := &tls.Config{
		RootCAs:    caCertPool,
		ServerName: "localhost",
		// No client cert!
	}

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := pb.NewOpAMPServiceClient(conn)
	stream, err := client.Connect(context.Background())
	if err != nil {
		// Connection refused at TLS level - this is expected
		return
	}

	// Try sending - should fail due to mTLS
	err = stream.Send(&pb.AgentToServer{InstanceUid: "bad-agent"})
	if err != nil {
		return // Expected
	}

	_, err = stream.Recv()
	if err == nil {
		t.Error("expected error when connecting without client cert")
	}
}
