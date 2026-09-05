package secureconn

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthenticatedSessionCarriesTrafficWithSharedSecret(t *testing.T) {
	serverAuth := newTestAuthenticator(t, "shared-secret")
	clientAuth := newTestAuthenticator(t, "shared-secret")
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	clientResult := make(chan sessionResult, 1)
	go func() {
		conn, err := clientAuth.Connect(clientRaw)
		clientResult <- sessionResult{conn: conn, err: err}
	}()

	serverConn, err := serverAuth.Accept(serverRaw)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	client := <-clientResult
	if client.err != nil {
		t.Fatalf("connect: %v", client.err)
	}

	want := []byte("authenticated input")
	writeErr := make(chan error, 1)
	go func() {
		_, err := client.conn.Write(want)
		writeErr <- err
	}()

	got := make([]byte, len(want))
	if _, err := io.ReadFull(serverConn, got); err != nil {
		t.Fatalf("read traffic: %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("write traffic: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("traffic = %q, want %q", got, want)
	}
}

func TestAuthenticatedSessionRejectsDifferentSecrets(t *testing.T) {
	serverAuth := newTestAuthenticator(t, "server-secret")
	clientAuth := newTestAuthenticator(t, "client-secret")
	serverRaw, clientRaw := net.Pipe()

	serverErr := make(chan error, 1)
	go func() {
		_, err := serverAuth.Accept(serverRaw)
		serverErr <- err
	}()

	if conn, err := clientAuth.Connect(clientRaw); err == nil {
		conn.Close()
		t.Fatal("connect succeeded with a different secret")
	}

	select {
	case err := <-serverErr:
		if err == nil {
			t.Fatal("accept succeeded with a different secret")
		}
	case <-time.After(time.Second):
		t.Fatal("accept did not stop after authentication failed")
	}
}

func TestLoadOrCreateSecretPersistsPrivatePairingCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "pairing.key")

	first, firstCode, err := LoadOrCreateSecret(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, secondCode, err := LoadOrCreateSecret(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if string(first) != string(second) || firstCode != secondCode {
		t.Fatal("pairing secret changed between loads")
	}
	if len(first) != 32 {
		t.Fatalf("secret length = %d, want 32", len(first))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat pairing key: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("pairing key mode = %o, want 600", got)
	}
}

func TestNewRejectsMissingPairingSecret(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("New accepted a missing pairing secret")
	}
}

type sessionResult struct {
	conn net.Conn
	err  error
}

func newTestAuthenticator(t *testing.T, label string) *Authenticator {
	t.Helper()
	secret := []byte(label + "-0123456789abcdef0123456789abcdef")
	auth, err := New(secret)
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	return auth
}
