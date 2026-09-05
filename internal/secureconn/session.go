// Package secureconn establishes encrypted, mutually authenticated transport
// sessions from a pre-shared secret.
package secureconn

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"time"
)

const (
	authTimeout = 5 * time.Second
	proofSize   = sha256.Size
)

var errAuthenticationFailed = errors.New("secureconn: authentication failed")

// Authenticator upgrades network connections to TLS 1.3 and authenticates
// both peers with a shared secret bound to the resulting TLS channel.
type Authenticator struct {
	secret []byte

	certOnce sync.Once
	cert     tls.Certificate
	certErr  error
}

// New constructs an Authenticator. Secrets must contain at least 128 bits of
// entropy; GenerateSecret returns the recommended representation.
func New(secret []byte) (*Authenticator, error) {
	if err := ValidateSecret(secret); err != nil {
		return nil, err
	}
	return &Authenticator{secret: bytes.Clone(secret)}, nil
}

// ValidateSecret checks whether a shared secret is safe to use.
func ValidateSecret(secret []byte) error {
	if len(secret) < 16 {
		return errors.New("secureconn: shared secret must be at least 16 bytes")
	}
	return nil
}

// GenerateSecret returns a random 256-bit secret.
func GenerateSecret() ([]byte, error) {
	secret := make([]byte, secretSize)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("secureconn: generate secret: %w", err)
	}
	return secret, nil
}

// Accept authenticates an inbound connection as the server peer.
func (a *Authenticator) Accept(raw net.Conn) (net.Conn, error) {
	cert, err := a.serverCertificate()
	if err != nil {
		raw.Close()
		return nil, err
	}
	conn := tls.Server(raw, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	})
	if err := authenticateServer(conn, a.secret); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Connect authenticates an outbound connection as the client peer.
func (a *Authenticator) Connect(raw net.Conn) (net.Conn, error) {
	conn := tls.Client(raw, &tls.Config{
		// The ephemeral certificate supplies TLS encryption. Peer identity is
		// verified immediately afterward by the channel-bound shared-secret
		// proof, so a public CA or stable hostname is neither needed nor used.
		InsecureSkipVerify: true, //nolint:gosec
		MinVersion:         tls.VersionTLS13,
	})
	if err := authenticateClient(conn, a.secret); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (a *Authenticator) serverCertificate() (tls.Certificate, error) {
	a.certOnce.Do(func() {
		a.cert, a.certErr = generateCertificate()
	})
	return a.cert, a.certErr
}

func authenticateServer(conn *tls.Conn, secret []byte) error {
	if err := conn.SetDeadline(time.Now().Add(authTimeout)); err != nil {
		return err
	}
	defer conn.SetDeadline(time.Time{})
	if err := conn.Handshake(); err != nil {
		return fmt.Errorf("secureconn: TLS handshake: %w", err)
	}
	binding, err := channelBinding(conn)
	if err != nil {
		return err
	}
	if _, err := conn.Write(proof(secret, binding, "server")); err != nil {
		return fmt.Errorf("secureconn: send server proof: %w", err)
	}
	got := make([]byte, proofSize)
	if _, err := io.ReadFull(conn, got); err != nil {
		return fmt.Errorf("secureconn: read client proof: %w", err)
	}
	if !hmac.Equal(got, proof(secret, binding, "client")) {
		return errAuthenticationFailed
	}
	return nil
}

func authenticateClient(conn *tls.Conn, secret []byte) error {
	if err := conn.SetDeadline(time.Now().Add(authTimeout)); err != nil {
		return err
	}
	defer conn.SetDeadline(time.Time{})
	if err := conn.Handshake(); err != nil {
		return fmt.Errorf("secureconn: TLS handshake: %w", err)
	}
	binding, err := channelBinding(conn)
	if err != nil {
		return err
	}
	got := make([]byte, proofSize)
	if _, err := io.ReadFull(conn, got); err != nil {
		return fmt.Errorf("secureconn: read server proof: %w", err)
	}
	if !hmac.Equal(got, proof(secret, binding, "server")) {
		return errAuthenticationFailed
	}
	if _, err := conn.Write(proof(secret, binding, "client")); err != nil {
		return fmt.Errorf("secureconn: send client proof: %w", err)
	}
	return nil
}

func channelBinding(conn *tls.Conn) ([]byte, error) {
	state := conn.ConnectionState()
	binding, err := state.ExportKeyingMaterial("universal-control/2", nil, 32)
	if err != nil {
		return nil, fmt.Errorf("secureconn: channel binding: %w", err)
	}
	return binding, nil
}

func proof(secret, binding []byte, role string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(binding)
	mac.Write([]byte(role))
	return mac.Sum(nil)
}

func generateCertificate() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("secureconn: generate key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("secureconn: generate serial: %w", err)
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "universal-control ephemeral"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("secureconn: create certificate: %w", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}, nil
}
