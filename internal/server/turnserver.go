package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pion/turn/v4"
)

func loadOrCreateTURNSecret(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return strings.TrimSpace(string(b)), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

func startTURN(publicIP string, port int, secret string) (*turn.Server, error) {
	if publicIP == "" {
		return nil, fmt.Errorf("TURN public IP required")
	}
	ip := net.ParseIP(publicIP)
	if ip == nil {
		return nil, fmt.Errorf("invalid TURN public IP %q", publicIP)
	}
	if port <= 0 {
		port = 3478
	}

	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		return nil, fmt.Errorf("TURN listen: %w", err)
	}

	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:       "connect",
		AuthHandler: turn.LongTermTURNRESTAuthHandler(secret, nil),
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: ip,
					Address:      "0.0.0.0",
				},
			},
		},
	})
	if err != nil {
		_ = udpListener.Close()
		return nil, err
	}
	return srv, nil
}
