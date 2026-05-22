package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
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

	mu        sync.Mutex
	known     map[string]Device
	running   bool
	cancelRun context.CancelFunc
	runDone   chan struct{}
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
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.running = true
	s.cancelRun = cancel
	s.runDone = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		s.run(runCtx)
		s.mu.Lock()
		s.running = false
		s.cancelRun = nil
		s.runDone = nil
		s.mu.Unlock()
	}()
}

func (s *Scanner) Stop() {
	s.mu.Lock()
	cancel := s.cancelRun
	done := s.runDone
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *Scanner) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
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

var priorityNames = []string{"physical", "vpn", "container", "docker", "other"}

func (s *Scanner) scan(ctx context.Context) {
	batches := discoveryEnumerateTargets()
	if len(batches) == 0 {
		log.Printf("[discovery] no targets to scan")
		return
	}

	total := 0
	for _, b := range batches {
		total += len(b.addrs)
	}
	log.Printf("[discovery] scanning %d targets across %d tiers", total, len(batches))

	found := 0
	var foundMu sync.Mutex
	for _, batch := range batches {
		select {
		case <-ctx.Done():
			return
		default:
		}
		name := "unknown"
		if batch.priority < len(priorityNames) {
			name = priorityNames[batch.priority]
		}
		log.Printf("[discovery] tier %d (%s): %d targets", batch.priority, name, len(batch.addrs))

		sem := make(chan struct{}, 128)
		var wg sync.WaitGroup
		for _, addr := range batch.addrs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(a netip.Addr) {
				defer wg.Done()
				defer func() { <-sem }()
				device, ok := discoveryProbeTarget(ctx, a)
				if !ok {
					return
				}
				foundMu.Lock()
				found++
				foundMu.Unlock()
				log.Printf("[discovery] found: %s (%s)", device.Name, device.BaseURL)
				s.publish(device)
			}(addr)
		}
		wg.Wait()
	}
	log.Printf("[discovery] scan complete, %d device(s) found", found)
}

var (
	discoveryEnumerateTargets = enumerateTargets
	discoveryProbeTarget      = probeTarget
)

// enumerateTargetsFlat is a convenience for tests that returns a single batch.
func enumerateTargetsFlat(addrs []netip.Addr) []targetBatch {
	if len(addrs) == 0 {
		return nil
	}
	return []targetBatch{{priority: 0, addrs: addrs}}
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

// interfacePriority classifies a network interface into scan priority tiers:
//   0 = physical (eth*, en*, eno*, wl*, wlan*, etc.)
//   1 = VPN / tunnel (tun*, tap*, wg*, tailscale*)
//   2 = containers / virtual (lxc*, lxd*, virbr*)
//   3 = docker (docker*, br-*, veth*)
//   4 = anything else
func interfacePriority(name string) int {
	for _, prefix := range []string{"docker", "br-", "veth"} {
		if strings.HasPrefix(name, prefix) {
			return 3
		}
	}
	for _, prefix := range []string{"lxc", "lxd", "virbr"} {
		if strings.HasPrefix(name, prefix) {
			return 2
		}
	}
	for _, prefix := range []string{"tun", "tap", "wg", "tailscale"} {
		if strings.HasPrefix(name, prefix) {
			return 1
		}
	}
	for _, prefix := range []string{"eth", "en", "wl", "wlan", "eno", "ens", "enp", "wlp"} {
		if strings.HasPrefix(name, prefix) {
			return 0
		}
	}
	return 4
}

type targetBatch struct {
	priority int
	addrs    []netip.Addr
}

func enumerateTargets() []targetBatch {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	buckets := map[int][]netip.Addr{}
	seen := map[netip.Addr]struct{}{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		prio := interfacePriority(iface.Name)
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
				buckets[prio] = append(buckets[prio], candidate)
			}
		}
	}

	var batches []targetBatch
	for prio := 0; prio <= 4; prio++ {
		addrs := buckets[prio]
		if len(addrs) == 0 {
			continue
		}
		slices.SortFunc(addrs, func(a, b netip.Addr) int {
			return strings.Compare(a.String(), b.String())
		})
		batches = append(batches, targetBatch{priority: prio, addrs: addrs})
	}
	return batches
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
	ctx, cancel := context.WithTimeout(parent, 600*time.Millisecond)
	defer cancel()

	baseURL := fmt.Sprintf("%s://%s", scheme, addr.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/device/status", nil)
	if err != nil {
		return Device{}, false
	}
	resp, err := discoveryHTTPClient.Do(req)
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

var discoveryHTTPClient = &http.Client{
	Timeout: 600 * time.Millisecond,
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: 600 * time.Millisecond,
	},
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
