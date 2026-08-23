// Command kairo is a transparent domain-based split router: DNS policy, DoH,
// DoT and an SNI proxy in a single binary.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alecthomas/kong"

	"kairo/internal/acme"
	"kairo/internal/config"
	"kairo/internal/database"
	"kairo/internal/logx"
	"kairo/internal/metrics"
	"kairo/internal/server"
	"kairo/internal/sni"
	"kairo/internal/state"
	"kairo/internal/version"
)

// CLI declares the command tree. Kong maps this struct directly onto the
// subcommands and their options, click-style.
type CLI struct {
	Run      RunCmd      `cmd:"" help:"Run the Kairo server (DNS, DoH/DoT, SNI router)."`
	Generate GenerateCmd `cmd:"" help:"Write a ready-to-edit config tree."`
	Migrate  MigrateCmd  `cmd:"" help:"Bring an existing config up to the current schema."`
	GenIPs   GenIPsCmd   `cmd:"" help:"Resolve the ip_source domains file into the allowlist and exit."`
	Version  VersionCmd  `cmd:"" help:"Print the Kairo version and exit."`
}

// configFlag is shared by commands that need a configuration file.
type configFlag struct {
	Config string `help:"Path to configuration file." default:"config.yaml"`
}

func main() {
	cli := CLI{}
	logx.Setup()

	ctx := kong.Parse(&cli,
		kong.Name("kairo"),
		kong.Description("Transparent domain-based split routing: DNS policy, DoH/DoT and an SNI proxy in one binary."),
		kong.UsageOnError(),
	)
	if err := ctx.Run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// seedAllowlistCache loads the durable allowlist (the union of all users'
// allowed IPs) into the in-memory cache and installs PersistIP so IPs
// generated from the ip-source file are stored under the admin account.
func seedAllowlistCache(st *state.State, db *database.DB, adminUserID int) {
	ips, err := db.DistinctAllowedIPs()
	if err != nil {
		slog.Error("seeding allowlist cache failed", "error", err)
	}
	st.SeedAllowed(ips)
	st.PersistIP = func(ip net.IP) error {
		_, err := db.AddUserIPIfAbsent(adminUserID, ip.String())
		return err
	}
}

// ---------------------------------------------------------------------------
// run
// ---------------------------------------------------------------------------

type RunCmd struct {
	configFlag
}

func (r *RunCmd) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load(r.Config)
	if err != nil {
		return err
	}

	// Initialize database
	dbPath := filepath.Join(cfg.DataDir, "kairo.db")
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Ensure admin user exists (auto-created from api_key)
	if err := db.EnsureAdmin("admin", cfg.AdminPassword); err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}
	slog.Info("admin user ready")

	admin, err := db.GetUserByUsername("admin")
	if err != nil || admin == nil {
		return fmt.Errorf("resolve admin user: %v", err)
	}

	// One-time 0.2.x data migration: import the old allowed.txt allowlist into
	// the admin account and retire the file as allowed.txt.legacy.
	if n, err := db.MigrateLegacyAllowedFile(cfg.DataDir, admin.ID); err != nil {
		return fmt.Errorf("migrate legacy allowlist: %w", err)
	} else if n > 0 {
		slog.Info("legacy IPs imported into admin account", "count", n)
	}

	// Clean up expired sessions periodically
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = db.ExpireOldSessions()
			case <-ctx.Done():
				return
			}
		}
	}()

	st, err := state.NewState(cfg)
	if err != nil {
		return err
	}
	seedAllowlistCache(st, db, admin.ID)

	go st.Watch(ctx)
	go st.RunGenerator(ctx)

	getCert, renew, err := certSource(ctx, cfg)
	if err != nil {
		return err
	}

	m := metrics.New(st.AllowedCount, st.RestrictedCount)
	srv := server.New(cfg, st, version.Version, m, getCert)
	srv.SetDB(db)
	handler := srv.BuildHandler()

	slog.Info("Kairo starting", "version", version.Version, "host", cfg.Host, "vps_ip", cfg.VPSIP)
	slog.Info("policy", "restricted", st.RestrictedCount(), "allowlisted", st.AllowedCount())

	go srv.StartDNS(cfg.Listen.DNS)
	go srv.StartDoT()
	go srv.StartHTTP()
	go srv.StartMetrics()
	go sni.Start(cfg, st, m, handler, getCert, db)
	if renew != nil {
		go renew(ctx)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	slog.Info("shutting down")
	return nil
}

// certSource builds the TLS certificate source for DoT and the SNI router. When
// ACME is configured it manages the certificate automatically via the http-01
// challenge; otherwise it falls back to the static tls.cert/tls.key files. A
// nil getCert means TLS termination is disabled. renew, when non-nil, runs the
// ACME background renewal loop.
func certSource(ctx context.Context, cfg *config.Config) (server.GetCertificate, func(context.Context), error) {
	if cfg.ACME.Email != "" && cfg.Host != "" {
		am, err := acme.New(cfg.Host, cfg.ACME)
		if err != nil {
			return nil, nil, err
		}
		if err := am.Ensure(ctx); err != nil {
			slog.Error("acme: initial certificate obtain failed, will retry on demand", "domain", cfg.Host, "error", err)
		}
		return am.GetCertificate, am.Run, nil
	}

	if cfg.TLS.Cert != "" && cfg.TLS.Key != "" {
		cer, err := tls.LoadX509KeyPair(cfg.TLS.Cert, cfg.TLS.Key)
		if err != nil {
			return nil, nil, fmt.Errorf("load tls certificate: %w", err)
		}
		cer.Leaf, _ = x509.ParseCertificate(cer.Certificate[0])
		getCert := func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return &cer, nil }
		return getCert, nil, nil
	}

	return nil, nil, nil
}

type GenIPsCmd struct {
	configFlag
}

func (g *GenIPsCmd) Run() error {
	cfg, err := config.Load(g.Config)
	if err != nil {
		return err
	}
	dbPath := filepath.Join(cfg.DataDir, "kairo.db")
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := db.EnsureAdmin("admin", cfg.AdminPassword); err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}
	admin, err := db.GetUserByUsername("admin")
	if err != nil || admin == nil {
		return fmt.Errorf("resolve admin user: %v", err)
	}

	st, err := state.NewState(cfg)
	if err != nil {
		return err
	}
	seedAllowlistCache(st, db, admin.ID)
	added, failed, err := st.GenerateIPs()
	if err != nil {
		return err
	}
	slog.Info("ip-source allowlist updated", "added", added, "unresolved", failed)
	return nil
}

type GenerateCmd struct {
	Kind string `arg:"" help:"What to generate. Only 'config' is supported."`
	Dir  string `arg:"" optional:"" help:"Directory to write into (default: current directory)."`
}

func (g *GenerateCmd) Run() error {
	return config.GenerateConfigs([]string{g.Kind, g.Dir})
}

type MigrateCmd struct {
	Path string `arg:"" optional:"" help:"Path to the config file (default: config.yaml)."`
}

func (m *MigrateCmd) Run() error {
	path := m.Path
	if path == "" {
		path = "config.yaml"
	}
	return config.Migrate(path)
}

type VersionCmd struct{}

func (v *VersionCmd) Run() error {
	slog.Info("Kairo", "version", version.Version)
	return nil
}
