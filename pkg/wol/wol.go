package wol

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

const magicPacketSize = 102

// ParseMAC accepts common MAC address formats (colon, dash, or bare hex)
// and returns a 6-byte hardware address.
func ParseMAC(s string) (net.HardwareAddr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty MAC address")
	}
	hw, err := net.ParseMAC(s)
	if err == nil && len(hw) == 6 {
		return hw, nil
	}
	// Try bare hex (e.g. "001122334455").
	bare := strings.ReplaceAll(strings.ReplaceAll(s, ":", ""), "-", "")
	if len(bare) == 12 {
		b, err := hex.DecodeString(bare)
		if err == nil && len(b) == 6 {
			return net.HardwareAddr(b), nil
		}
	}
	return nil, fmt.Errorf("invalid MAC address: %s", s)
}

// FormatMAC returns the canonical colon-separated lowercase representation.
func FormatMAC(hw net.HardwareAddr) string {
	return hw.String()
}

// Send broadcasts a Wake-on-LAN magic packet for the given MAC address.
// The packet is sent as a UDP broadcast on port 9.
func Send(mac net.HardwareAddr) error {
	if len(mac) != 6 {
		return fmt.Errorf("MAC address must be 6 bytes, got %d", len(mac))
	}
	var packet [magicPacketSize]byte
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac)
	}
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: 9,
	})
	if err != nil {
		return fmt.Errorf("dial broadcast: %w", err)
	}
	defer conn.Close()
	_, err = conn.Write(packet[:])
	if err != nil {
		return fmt.Errorf("send magic packet: %w", err)
	}
	return nil
}

// IsValidMAC returns true if s can be parsed as a 6-byte MAC address.
func IsValidMAC(s string) bool {
	_, err := ParseMAC(s)
	return err == nil
}
