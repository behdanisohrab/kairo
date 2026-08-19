// Package acme manages a TLS certificate for the host name automatically via
// the ACME protocol (Let's Encrypt), using github.com/go-acme/lego with the
// http-01 challenge. It replaces hand-managed certificates: the certificate is
// obtained on first run and renewed automatically before it expires.
package acme

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"kairo/internal/config"
)

const (
	accountKeyFile = "account.key"
	accountFile    = "account.json"
	certFile       = "cert.pem"
	keyFile        = "key.pem"

	renewCheckInterval = 6 * time.Hour
)

// account implements registration.User and is persisted across restarts so the
// ACME account and its private key survive a process restart.
type account struct {
	Email        string                 `json:"email"`
	Registration *registration.Resource `json:"registration"`
	key          crypto.PrivateKey
}

func (a *account) GetEmail() string                        { return a.Email }
func (a *account) GetRegistration() *registration.Resource { return a.Registration }
func (a *account) GetPrivateKey() crypto.PrivateKey        { return a.key }

// Manager owns the ACME client, the on-disk certificate store and the currently
// active certificate. It is safe for concurrent use.
type Manager struct {
	domain      string
	email       string
	storage     string
	directory   string
	renewBefore time.Duration
	httpListen  string

	mu       sync.RWMutex
	cert     *tls.Certificate
	privKey  crypto.PrivateKey
	client   *lego.Client
	acct     *account
	ensureMu sync.Mutex
}

// New builds a Manager for the given host domain using the ACME config. It
// creates the storage directory and (re)loads or registers the ACME account,
// but does not touch the network until Ensure or GetCertificate is called.
func New(domain string, cfg config.ACMEConfig) (*Manager, error) {
	directory := cfg.Directory
	if directory == "" {
		directory = lego.LEDirectoryProduction
	}
	renewBefore := time.Duration(cfg.RenewBeforeDays) * 24 * time.Hour
	if renewBefore <= 0 {
		renewBefore = 30 * 24 * time.Hour
	}
	httpListen := cfg.HTTPListen
	if httpListen == "" {
		httpListen = ":80"
	}

	m := &Manager{
		domain:      domain,
		email:       cfg.Email,
		storage:     cfg.Storage,
		directory:   directory,
		renewBefore: renewBefore,
		httpListen:  httpListen,
	}

	if err := os.MkdirAll(m.storage, 0o700); err != nil {
		return nil, fmt.Errorf("create acme storage %s: %w", m.storage, err)
	}
	if err := m.setupClient(); err != nil {
		return nil, err
	}
	return m, nil
}

// setupClient loads or creates the ACME account and builds the lego client
// with the http-01 challenge provider wired up.
func (m *Manager) setupClient() error {
	acct, err := m.loadAccount()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load acme account: %w", err)
	}
	if acct == nil {
		key, err := certcrypto.GeneratePrivateKey(certcrypto.EC256)
		if err != nil {
			return fmt.Errorf("generate account key: %w", err)
		}
		acct = &account{Email: m.email, key: key}
	}
	m.acct = acct

	lc := lego.NewConfig(acct)
	lc.CADirURL = m.directory
	lc.Certificate.KeyType = certcrypto.EC256
	client, err := lego.NewClient(lc)
	if err != nil {
		return fmt.Errorf("new acme client: %w", err)
	}

	if m.acct.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return fmt.Errorf("register acme account: %w", err)
		}
		m.acct.Registration = reg
		if err := m.saveAccount(); err != nil {
			return err
		}
	}

	host, port := splitHostPort(m.httpListen)
	client.Challenge.SetHTTP01Provider(http01.NewProviderServer(host, port))
	m.client = client
	return nil
}

// Ensure makes a certificate available: it loads the one already stored, or
// obtains a fresh one via the http-01 challenge. Safe to call repeatedly.
func (m *Manager) Ensure(ctx context.Context) error {
	m.ensureMu.Lock()
	defer m.ensureMu.Unlock()

	m.mu.RLock()
	if m.cert != nil {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()

	if err := m.loadStored(); err == nil {
		slog.Info("acme: using stored certificate", "domain", m.domain, "path", m.storage)
		return nil
	}

	slog.Info("acme: obtaining certificate", "domain", m.domain, "email", m.email)
	privKey, err := certcrypto.GeneratePrivateKey(certcrypto.EC256)
	if err != nil {
		return fmt.Errorf("generate certificate key: %w", err)
	}
	res, err := m.obtain(ctx, privKey)
	if err != nil {
		return fmt.Errorf("obtain certificate: %w", err)
	}
	return m.storeResult(res)
}

// GetCertificate returns the current certificate, obtaining it on first use if
// Ensure has not run yet. It satisfies tls.Config.GetCertificate.
func (m *Manager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.RLock()
	cert := m.cert
	m.mu.RUnlock()
	if cert != nil {
		return cert, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := m.Ensure(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cert, nil
}

// Run periodically checks whether the certificate needs renewal and renews it
// when it is within renewBefore of expiring. It blocks until ctx is done.
func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(renewCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.NeedsRenewal() {
				slog.Info("acme: certificate nearing expiry, renewing", "domain", m.domain)
				if err := m.Renew(ctx); err != nil {
					slog.Error("acme: renewal failed", "domain", m.domain, "error", err)
				}
			}
		}
	}
}

// NeedsRenewal reports whether the active certificate expires within
// renewBefore, or whether no certificate is loaded yet.
func (m *Manager) NeedsRenewal() bool {
	m.mu.RLock()
	cert := m.cert
	m.mu.RUnlock()
	if cert == nil {
		return true
	}
	if cert.Leaf == nil {
		return true
	}
	return time.Until(cert.Leaf.NotAfter) < m.renewBefore
}

// Renew obtains a fresh certificate reusing the existing private key and swaps
// it into place atomically.
func (m *Manager) Renew(ctx context.Context) error {
	m.ensureMu.Lock()
	defer m.ensureMu.Unlock()

	m.mu.RLock()
	privKey := m.privKey
	m.mu.RUnlock()
	if privKey == nil {
		return m.Ensure(ctx)
	}

	res, err := m.obtain(ctx, privKey)
	if err != nil {
		return fmt.Errorf("renew certificate: %w", err)
	}
	return m.storeResult(res)
}

// Close is a no-op kept for symmetry; nothing holds the challenge listener open
// between calls.
func (m *Manager) Close() {}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

// obtain asks the ACME server for a certificate covering the manager's domain,
// reusing privKey when it is non-nil so renewals keep the same key pair.
func (m *Manager) obtain(_ context.Context, privKey crypto.PrivateKey) (*certificate.Resource, error) {
	return m.client.Certificate.Obtain(certificate.ObtainRequest{
		Domains:    []string{m.domain},
		PrivateKey: privKey,
		Bundle:     true,
	})
}

// storeResult writes the issued certificate and key to disk and swaps them into
// the active position.
func (m *Manager) storeResult(res *certificate.Resource) error {
	if err := os.WriteFile(m.path(certFile), res.Certificate, 0o600); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}
	if err := os.WriteFile(m.path(keyFile), res.PrivateKey, 0o600); err != nil {
		return fmt.Errorf("write certificate key: %w", err)
	}
	cer, err := tls.X509KeyPair(res.Certificate, res.PrivateKey)
	if err != nil {
		return fmt.Errorf("load obtained certificate: %w", err)
	}
	cer.Leaf, _ = x509.ParseCertificate(cer.Certificate[0])

	m.mu.Lock()
	m.cert = &cer
	m.privKey, _ = certcrypto.ParsePEMPrivateKey(res.PrivateKey)
	m.mu.Unlock()
	slog.Info("acme: certificate ready", "domain", m.domain)
	return nil
}

// loadStored loads the certificate previously written to disk, if any.
func (m *Manager) loadStored() error {
	certPEM, err := os.ReadFile(m.path(certFile))
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(m.path(keyFile))
	if err != nil {
		return err
	}
	cer, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("load stored certificate: %w", err)
	}
	cer.Leaf, _ = x509.ParseCertificate(cer.Certificate[0])
	privKey, err := certcrypto.ParsePEMPrivateKey(keyPEM)
	if err != nil {
		return fmt.Errorf("parse stored key: %w", err)
	}
	m.mu.Lock()
	m.cert = &cer
	m.privKey = privKey
	m.mu.Unlock()
	return nil
}

// loadAccount reads the persisted account, if present.
func (m *Manager) loadAccount() (*account, error) {
	keyPEM, err := os.ReadFile(m.path(accountKeyFile))
	if err != nil {
		return nil, err
	}
	key, err := certcrypto.ParsePEMPrivateKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse account key: %w", err)
	}
	a := &account{key: key}
	if data, err := os.ReadFile(m.path(accountFile)); err == nil {
		_ = json.Unmarshal(data, a)
		a.key = key
	}
	return a, nil
}

// saveAccount writes the account key and registration to disk.
func (m *Manager) saveAccount() error {
	keyPEM := certcrypto.PEMEncode(m.acct.GetPrivateKey())
	if err := os.WriteFile(m.path(accountKeyFile), keyPEM, 0o600); err != nil {
		return fmt.Errorf("write account key: %w", err)
	}
	data, err := json.Marshal(m.acct)
	if err != nil {
		return fmt.Errorf("encode account: %w", err)
	}
	if err := os.WriteFile(m.path(accountFile), data, 0o600); err != nil {
		return fmt.Errorf("write account: %w", err)
	}
	return nil
}

func (m *Manager) path(name string) string { return filepath.Join(m.storage, name) }

// splitHostPort splits an address like ":80" or "0.0.0.0:80" into the interface
// and port strings lego's HTTP-01 provider expects.
func splitHostPort(addr string) (host, port string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", strings.TrimPrefix(addr, ":")
	}
	return host, port
}
