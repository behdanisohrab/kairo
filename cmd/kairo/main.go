// Command kairo is a transparent domain-based split router: DNS policy, DoH,
// DoT and an SNI proxy in a single binary.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"

	"kairo/internal/config"
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

	st, err := state.NewState(cfg)
	if err != nil {
		return err
	}

	go st.Watch(ctx)
	go st.RunGenerator(ctx)

	m := metrics.New(st.AllowedCount, st.RestrictedCount)
	srv := server.New(cfg, st, version.Version, m)
	handler := srv.BuildHandler()

	slog.Info("Kairo starting", "version", version.Version, "host", cfg.Host, "vps_ip", cfg.VPSIP)
	slog.Info("policy", "restricted", st.RestrictedCount(), "allowlisted", st.AllowedCount())

	go srv.StartDNS(cfg.Listen.DNS)
	go srv.StartDoT()
	go srv.StartHTTP()
	go srv.StartMetrics()
	go sni.Start(cfg, st, m, handler)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	slog.Info("shutting down")
	return nil
}

type GenIPsCmd struct {
	configFlag
}

func (g *GenIPsCmd) Run() error {
	cfg, err := config.Load(g.Config)
	if err != nil {
		return err
	}
	st, err := state.NewState(cfg)
	if err != nil {
		return err
	}
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
