package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

var (
	cfg       *Config
	startTime = time.Now()

	dnsLimiter *rate.Limiter
	apiLimiter *rate.Limiter
)

func main() {
	configPath := flag.String("config", "config.json", "path to configuration file")
	genIPs := flag.Bool("gen-ips", false, "resolve the ip_source domains file into the allowlist and exit")
	generate := flag.Bool("generate", false, "write default config files and exit, e.g. --generate config configs/")
	showVersion := flag.Bool("version", false, "print the Kairo version and exit")
	flag.Parse()

	if *showVersion {
		log.Printf("Kairo %s", version)
		return
	}

	if *generate {
		if err := generateConfigs(flag.Args()); err != nil {
			log.Fatalf("kairo: %v", err)
		}
		return
	}

	var err error
	cfg, err = LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("kairo: %v", err)
	}

	dnsLimiter = rate.NewLimiter(rate.Limit(cfg.Rate.DNS), cfg.Rate.DNSBurst)
	apiLimiter = rate.NewLimiter(rate.Limit(cfg.Rate.API), cfg.Rate.APIBurst)

	state, err := NewState(cfg)
	if err != nil {
		log.Fatalf("kairo: %v", err)
	}

	if *genIPs {
		added, failed, err := state.GenerateIPs()
		if err != nil {
			log.Fatalf("kairo: ip-source generation failed: %v", err)
		}
		log.Printf("kairo: ip-source: %d new addresses, %d unresolved", added, failed)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go state.Watch(ctx)
	go state.RunGenerator(ctx)

	log.Printf("Kairo %s starting (host=%q vps_ip=%s)", version, cfg.Host, cfg.VPSIP)
	log.Printf("restricted domains: %d, allowlisted clients: %d", state.RestrictedCount(), state.AllowedCount())

	handler := buildHandler(state)

	go StartDNS(cfg.Listen.DNS, state)
	go StartDoT(state)
	go StartHTTP(state)
	go StartSNI(state, handler)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("Kairo shutting down")
}

// normalizeDomain strips the noise off a domain: lowercase, no trailing dots.
func normalizeDomain(d string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(d)), ".")
}

func remoteIPAddr(addr net.Addr) net.IP {
	if addr == nil {
		return nil
	}
	return parseHostIP(addr.String())
}

// parseHostIP pulls the IP out of "host" or "host:port".
func parseHostIP(hostport string) net.IP {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return net.ParseIP(hostport)
	}
	return net.ParseIP(host)
}
