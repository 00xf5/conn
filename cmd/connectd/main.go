package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"connect/internal/server"
)

func detectPublicURL(addr, configured string, tlsEnabled bool) (string, string) {
	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}

	if configured != "" && !strings.Contains(configured, "localhost") && !strings.Contains(configured, "127.0.0.1") {
		host := serverPublicHost(configured)
		return configured, host
	}

	port := strings.TrimPrefix(addr, ":")
	if port == "" {
		port = "8787"
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return configured, serverPublicHost(configured)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ip := ipNet.IP.String()
			if strings.HasPrefix(ip, "169.254.") {
				continue
			}
			return scheme + "://" + ip + ":" + port, ip
		}
	}
	return configured, serverPublicHost(configured)
}

func serverPublicHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "127.0.0.1"
	}
	return u.Hostname()
}

func configuredPublicURL(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	for _, key := range []string{"CONNECT_PUBLIC_URL", "RENDER_EXTERNAL_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func listenAddr(flagAddr string) string {
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		if strings.HasPrefix(p, ":") {
			return p
		}
		return ":" + p
	}
	if flagAddr != "" {
		return flagAddr
	}
	return ":8787"
}

func envBool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

func main() {
	defaultAddr := listenAddr("")
	addr := flag.String("addr", defaultAddr, "listen address (overridden by PORT env on Render)")
	publicURL := flag.String("public-url", configuredPublicURL(""), "public HTTPS base URL for viewer links (CONNECT_PUBLIC_URL or RENDER_EXTERNAL_URL)")
	keyPath := flag.String("key", "data/server.key", "Ed25519 private key path")
	dbPath := flag.String("db", "data/connect.db", "SQLite path for tenants / access accounts")
	tlsCert := flag.String("tls-cert", "data/tls.crt", "TLS certificate path")
	tlsKey := flag.String("tls-key", "data/tls.key", "TLS private key path")
	noTLS := flag.Bool("no-tls", envBool("CONNECT_NO_TLS"), "disable TLS (use behind Render/nginx TLS termination)")
	turnPort := flag.Int("turn-port", 3478, "embedded STUN/TURN UDP port (LAN only)")
	noTurn := flag.Bool("no-turn", envBool("CONNECT_NO_TURN"), "disable embedded STUN/TURN (required on Render)")
	requireTenant := flag.Bool("require-tenant", envBool("CONNECT_REQUIRE_TENANT"), "reject agents without tenantId")
	agentDir := flag.String("agent-dir", strings.TrimSpace(os.Getenv("CONNECT_AGENT_DIR")), "directory with agent.zip for /download + /install (default data/agent)")
	flag.Parse()
	if *agentDir == "" {
		*agentDir = "data/agent"
	}

	*addr = listenAddr(*addr)

	tlsEnabled := !*noTLS
	publicHost := ""
	if *publicURL == "" {
		*publicURL = "http://localhost:8787"
	}
	*publicURL, publicHost = detectPublicURL(*addr, *publicURL, tlsEnabled)
	if tlsEnabled {
		if strings.HasPrefix(*publicURL, "http://") {
			*publicURL = "https://" + strings.TrimPrefix(*publicURL, "http://")
		}
		if err := server.EnsureTLSCert(*tlsCert, *tlsKey, publicHost); err != nil {
			log.Fatalf("TLS cert: %v", err)
		}
	}

	iceCfg := server.LoadICEConfigFromEnv()
	var overrideICE []server.ICEServer
	if raw := strings.TrimSpace(os.Getenv("CONNECT_ICE_SERVERS")); raw != "" {
		parsed, err := server.ParseICEServersJSON(raw)
		if err != nil {
			log.Fatalf("CONNECT_ICE_SERVERS: %v", err)
		}
		overrideICE = parsed
	}

	srv, err := server.New(server.Config{
		Addr:               *addr,
		PublicURL:          *publicURL,
		PublicHost:         publicHost,
		KeyPath:            *keyPath,
		DBPath:             *dbPath,
		AdminToken:         strings.TrimSpace(os.Getenv("CONNECT_ADMIN_TOKEN")),
		RequireTenant:      *requireTenant,
		TURNPort:           *turnPort,
		EnableTURN:         !*noTurn,
		ICE:                iceCfg,
		OverrideICEServers: overrideICE,
		AgentDir:           *agentDir,
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("connectd listening on %s (%s)", *addr, *publicURL)
	log.Printf("server public key: %s", srv.PublicKey())
	log.Printf("admin: %s/admin/", *publicURL)
	log.Printf("dashboard: %s/dashboard/", *publicURL)
	log.Printf("viewer example: %s/v/{code}", *publicURL)
	log.Printf("host install: %s/install (agent dir %s)", *publicURL, *agentDir)
	if *noTurn {
		log.Printf("connectd: embedded TURN disabled")
	}
	if iceCfg.ExternalTURNURL != "" {
		log.Printf("connectd: external TURN %s (STUN: %v)", iceCfg.ExternalTURNURL, iceCfg.DefaultSTUN)
	} else if len(overrideICE) == 0 {
		log.Printf("connectd: no external TURN — LAN/direct only unless viewers share network with host")
	}

	var ln net.Listener
	if tlsEnabled {
		cert, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatalf("TLS load: %v", err)
		}
		ln, err = tls.Listen("tcp", *addr, &tls.Config{Certificates: []tls.Certificate{cert}})
	} else {
		ln, err = net.Listen("tcp", *addr)
	}
	if err != nil {
		log.Fatal(err)
	}
	if err := http.Serve(ln, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
