package storage

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewWithCustomCABundle(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(bundle, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CA_BUNDLE", bundle)

	s, err := New(context.Background(), Options{
		AccessKey: "key", SecretKey: "secret", ServiceURL: "https://s3.example.com", Region: "auto",
		Bucket: "bucket", ForcePathStyle: true, VerifySSL: true, CDNBaseURL: "https://cdn.example.com/",
	})
	if err != nil {
		t.Fatalf("New with AWS_CA_BUNDLE: %v", err)
	}
	if got := s.PublicURL("1.png"); got != "https://cdn.example.com/1.png" {
		t.Fatalf("PublicURL = %q", got)
	}
}

func TestNewRejectsIncompleteConfig(t *testing.T) {
	if _, err := New(context.Background(), Options{AccessKey: "k"}); err == nil {
		t.Fatal("expected an error for incomplete configuration")
	}
}
