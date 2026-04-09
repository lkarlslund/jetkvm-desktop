package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

type Device struct {
	Name      string
	BaseURL   string
	Host      string
	IP        string
	Scheme    string
	IsSetup   bool
	UpdatedAt time.Time
}

type Scanner struct {
	updates chan Device

	mu    sync.Mutex
	known map[string]Device
}

func NewScanner() *Scanner {
	return &Scanner{
		updates: make(chan Device, 64),
		known:   map[string]Device{},
	}
}

func (s *Scanner) Updates() <-chan Device {
	return s.updates
}

func (s *Scanner) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *Scanner) run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	s.scan(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scan(ctx)
		}
	}
}

func (s *Scanner) scan(ctx context.Context) {
	targets := enumerateTargets()
	if len(targets) == 0 {
		return
	}

	sem := make(chan struct{}, 48)
	var wg sync.WaitGroup
	for _, target := range targets {
		select {
		case <-ctx.Done():
			return
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(addr netip.Addr) {
			defer wg.Done()
			defer func() { <-sem }()
			device, ok := probeTarget(ctx, addr)
			if !ok {
				return
			}
			s.publish(device)
		}(target)
	}
	wg.Wait()
}

func (s *Scanner) publish(device Device) {
	s.mu.Lock()
	prev, exists := s.known[device.BaseURL]
	if exists && prev.Name == device.Name && prev.Host == device.Host && prev.IsSetup == device.IsSetup {
		s.known[device.BaseURL] = device
		s.mu.Unlock()
		return
	}
	s.known[device.BaseURL] = device
	s.mu.Unlock()

	select {
	case s.updates <- device:
	default:
	}
}

func enumerateTargets() []netip.Addr {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	seen := map[netip.Addr]struct{}{}
	out := make([]netip.Addr, 0, 256)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			prefix, ok := addrToPrefix(addr)
			if !ok || !prefix.Addr().Is4() {
				continue
			}
			for _, candidate := range prefixTargets(prefix) {
				if _, exists := seen[candidate]; exists {
					continue
				}
				seen[candidate] = struct{}{}
				out = append(out, candidate)
			}
		}
	}
	slices.SortFunc(out, func(a, b netip.Addr) int {
		return strings.Compare(a.String(), b.String())
	})
	return out
}

func addrToPrefix(addr net.Addr) (netip.Prefix, bool) {
	ipNet, ok := addr.(*net.IPNet)
	if !ok {
		return netip.Prefix{}, false
	}
	ip, ok := netip.AddrFromSlice(ipNet.IP)
	if !ok {
		return netip.Prefix{}, false
	}
	bits, _ := ipNet.Mask.Size()
	return netip.PrefixFrom(ip.Unmap(), bits), true
}

func prefixTargets(prefix netip.Prefix) []netip.Addr {
	prefix = prefix.Masked()
	addr := prefix.Addr()
	if !addr.Is4() {
		return nil
	}
	bits := prefix.Bits()
	if bits < 24 {
		bits = 24
		prefix = netip.PrefixFrom(addr, bits).Masked()
	}
	if bits > 30 {
		return nil
	}

	start := prefix.Addr().As4()
	hostBits := 32 - bits
	limit := uint32(1) << hostBits
	if limit > 256 {
		limit = 256
	}

	local := binaryIPv4(start)
	out := make([]netip.Addr, 0, limit)
	for i := uint32(1); i+1 < limit; i++ {
		candidate := netip.AddrFrom4(uint32ToIPv4(local + i))
		out = append(out, candidate)
	}
	return out
}

func binaryIPv4(value [4]byte) uint32 {
	return uint32(value[0])<<24 | uint32(value[1])<<16 | uint32(value[2])<<8 | uint32(value[3])
}

func uint32ToIPv4(value uint32) [4]byte {
	return [4]byte{
		byte(value >> 24),
		byte(value >> 16),
		byte(value >> 8),
		byte(value),
	}
}

func probeTarget(parent context.Context, addr netip.Addr) (Device, bool) {
	for _, scheme := range []string{"http", "https"} {
		device, ok := probeScheme(parent, addr, scheme)
		if ok {
			return device, true
		}
	}
	return Device{}, false
}

func probeScheme(parent context.Context, addr netip.Addr, scheme string) (Device, bool) {
	ctx, cancel := context.WithTimeout(parent, 1200*time.Millisecond)
	defer cancel()

	baseURL := fmt.Sprintf("%s://%s", scheme, addr.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/device/status", nil)
	if err != nil {
		return Device{}, false
	}
	client := &http.Client{
		Timeout: 1200 * time.Millisecond,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return Device{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Device{}, false
	}
	var status struct {
		IsSetup bool `json:"isSetup"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return Device{}, false
	}

	name := addr.String()
	host := reverseLookup(addr)
	if host != "" {
		name = host
	}

	return Device{
		Name:      name,
		BaseURL:   baseURL,
		Host:      host,
		IP:        addr.String(),
		Scheme:    scheme,
		IsSetup:   status.IsSetup,
		UpdatedAt: time.Now(),
	}, true
}

func reverseLookup(addr netip.Addr) string {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	type result struct {
		names []string
	}
	ch := make(chan result, 1)
	go func() {
		names, _ := net.LookupAddr(addr.String())
		ch <- result{names: names}
	}()

	select {
	case <-ctx.Done():
		return ""
	case res := <-ch:
		if len(res.names) == 0 {
			return ""
		}
		return strings.TrimSuffix(res.names[0], ".")
	}
}
