package socket

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func generateTestingSelfSignedCert(t *testing.T) (certPEM, keyPEM []byte) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(t, err, "failed to generate ed25519 key")

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().UTC().Add(-time.Hour),
		NotAfter:              time.Now().UTC().Add(time.Hour * 24),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, priv)
	assert.NoError(t, err)

	certBuf := &bytes.Buffer{}
	keyBuf := &bytes.Buffer{}

	assert.NoError(t, pem.Encode(certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}))
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	assert.NoError(t, err)
	assert.NoError(t, pem.Encode(keyBuf, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	return certBuf.Bytes(), keyBuf.Bytes()
}

func startMockServer(t *testing.T, useTLS bool, handler func(net.Conn)) (addr string, stop func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err, "failed to start mock server")
	t.Logf("started mock server at %s\n", ln.Addr().String())

	if useTLS {
		cert, key := generateTestingSelfSignedCert(t)

		certPair, err := tls.X509KeyPair(cert, key)
		assert.NoError(t, err, "failed to create x509 key pair")

		ln = tls.NewListener(ln, &tls.Config{
			Certificates: []tls.Certificate{certPair},
		})
		t.Logf("upgraded to tls at %s\n", ln.Addr().String())
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			t.Logf("accepted connection from %s\n", conn.RemoteAddr().String())
			go handler(conn)
		}
	}()

	return ln.Addr().String(), func() { _ = ln.Close() }
}
