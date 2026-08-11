// Package netdisc makes the server reachable by name instead of by
// "localhost:port".
//
// It answers two name resolution protocols that every modern Windows, macOS
// and Linux desktop already speaks, with no client-side install:
//
//	mDNS  (224.0.0.251:5353) -> http://kisaf.local
//	LLMNR (224.0.0.252:5355) -> http://kisaf        (Windows)
//
// Both are tiny DNS-over-multicast dialects, so a single hand-rolled encoder
// covers them and the binary keeps its zero-dependency build.
package netdisc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	mdnsPort  = 5353
	llmnrPort = 5355

	typeA   = 1
	typeANY = 255

	classIN         = 1
	classMask       = 0x7fff
	unicastRespBit  = 0x8000 // QU bit in an mDNS question
	cacheFlushBit   = 0x8000 // in an mDNS answer's class field
	flagResponseAA  = 0x8400 // QR + Authoritative Answer
	flagLLMNRAnswer = 0x8000 // QR only
)

var (
	mdnsGroup  = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: mdnsPort}
	llmnrGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 252), Port: llmnrPort}
)

// Responder announces one hostname on the local link.
type Responder struct {
	host string // "kisaf", without the .local suffix

	mu      sync.Mutex
	conns   []*net.UDPConn
	closed  bool
	stopped chan struct{}
	logf    func(string, ...any)
}

// Start begins answering queries for host and host.local. It never returns a
// hard error: name discovery is a convenience, and the server must stay usable
// on a machine where port 5353 is already taken by Bonjour or avahi.
func Start(host string, logf func(string, ...any)) *Responder {
	if logf == nil {
		logf = log.Printf
	}
	r := &Responder{
		host:    strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), ".local")),
		stopped: make(chan struct{}),
		logf:    logf,
	}
	if r.host == "" {
		r.host = "kisaf"
	}

	mdnsConns := r.listenGroup(mdnsGroup)
	llmnrConns := r.listenGroup(llmnrGroup)

	if len(mdnsConns) == 0 && len(llmnrConns) == 0 {
		r.logf("network discovery unavailable; http://%s.local will not resolve (use the IP address)", r.host)
		return r
	}

	for _, c := range mdnsConns {
		go r.serve(c, protoMDNS)
	}
	for _, c := range llmnrConns {
		go r.serve(c, protoLLMNR)
	}

	// Unsolicited announcements prime the neighbours' caches so the very first
	// browser request already resolves.
	go r.announce(mdnsConns)
	return r
}

type proto int

const (
	protoMDNS proto = iota
	protoLLMNR
)

// listenGroup joins the multicast group on every usable interface. One socket
// per interface is what makes this work on machines with Wi-Fi plus Ethernet
// plus a pile of virtual adapters.
func (r *Responder) listenGroup(group *net.UDPAddr) []*net.UDPConn {
	var conns []*net.UDPConn

	ifaces, err := net.Interfaces()
	if err != nil {
		ifaces = nil
	}
	for i := range ifaces {
		ifi := ifaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 {
			continue
		}
		if !hasIPv4(&ifi) {
			continue
		}
		conn, err := net.ListenMulticastUDP("udp4", &ifi, group)
		if err != nil {
			continue
		}
		_ = conn.SetReadBuffer(65536)
		conns = append(conns, conn)
	}

	if len(conns) == 0 {
		// Fall back to whatever interface the OS considers default.
		if conn, err := net.ListenMulticastUDP("udp4", nil, group); err == nil {
			conns = append(conns, conn)
		}
	}

	r.mu.Lock()
	r.conns = append(r.conns, conns...)
	r.mu.Unlock()
	return conns
}

func hasIPv4(ifi *net.Interface) bool {
	addrs, err := ifi.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
			return true
		}
	}
	return false
}

func (r *Responder) serve(conn *net.UDPConn, p proto) {
	buf := make([]byte, 9000)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			r.mu.Lock()
			closed := r.closed
			r.mu.Unlock()
			if closed {
				return
			}
			// A transient read error on one adapter should not kill the loop.
			time.Sleep(200 * time.Millisecond)
			continue
		}
		r.handle(conn, p, src, buf[:n])
	}
}

func (r *Responder) handle(conn *net.UDPConn, p proto, src *net.UDPAddr, packet []byte) {
	id, flags, questions, err := parseQuery(packet)
	if err != nil || flags&0x8000 != 0 { // ignore responses, only answer queries
		return
	}

	for _, q := range questions {
		if q.qtype != typeA && q.qtype != typeANY {
			continue
		}
		if !r.matches(p, q.name) {
			continue
		}
		ip := localIPFor(src.IP)
		if ip == nil {
			continue
		}

		var resp []byte
		dst := src
		if p == protoLLMNR {
			// LLMNR replies are always unicast and echo the question back.
			resp = buildAnswer(id, q, ip, 30, flagLLMNRAnswer, classIN, true)
		} else {
			resp = buildAnswer(0, q, ip, 120, flagResponseAA, classIN|cacheFlushBit, false)
			if q.qclass&unicastRespBit == 0 && src.Port == mdnsPort {
				dst = mdnsGroup
			}
		}
		if _, err := conn.WriteToUDP(resp, dst); err != nil {
			r.logf("could not send discovery reply: %v", err)
		}
	}
}

// matches decides whether a queried name is ours. mDNS asks for "kisaf.local",
// LLMNR asks for the bare "kisaf".
func (r *Responder) matches(p proto, name string) bool {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if p == protoLLMNR {
		return name == r.host
	}
	return name == r.host+".local"
}

func (r *Responder) announce(conns []*net.UDPConn) {
	q := question{name: r.host + ".local.", qtype: typeA, qclass: classIN}
	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-r.stopped:
			return
		case <-time.After(time.Duration(attempt) * time.Second):
		}
		for _, conn := range conns {
			ip := primaryIPv4()
			if ip == nil {
				continue
			}
			msg := buildAnswer(0, q, ip, 120, flagResponseAA, classIN|cacheFlushBit, false)
			_, _ = conn.WriteToUDP(msg, mdnsGroup)
		}
	}
}

// Close stops answering and releases the sockets.
func (r *Responder) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	conns := r.conns
	r.conns = nil
	close(r.stopped)
	r.mu.Unlock()

	for _, c := range conns {
		_ = c.Close()
	}
}

// ---------------------------------------------------------------------------
// Minimal DNS wire format
// ---------------------------------------------------------------------------

type question struct {
	name   string
	qtype  uint16
	qclass uint16
}

func parseQuery(msg []byte) (id uint16, flags uint16, questions []question, err error) {
	if len(msg) < 12 {
		return 0, 0, nil, errors.New("short packet")
	}
	id = binary.BigEndian.Uint16(msg[0:2])
	flags = binary.BigEndian.Uint16(msg[2:4])
	qdCount := int(binary.BigEndian.Uint16(msg[4:6]))
	if qdCount == 0 || qdCount > 32 {
		return id, flags, nil, nil
	}

	offset := 12
	for i := 0; i < qdCount; i++ {
		name, next, err := readName(msg, offset)
		if err != nil {
			return id, flags, questions, err
		}
		if next+4 > len(msg) {
			return id, flags, questions, errors.New("truncated question")
		}
		questions = append(questions, question{
			name:   name,
			qtype:  binary.BigEndian.Uint16(msg[next : next+2]),
			qclass: binary.BigEndian.Uint16(msg[next+2 : next+4]),
		})
		offset = next + 4
	}
	return id, flags, questions, nil
}

// readName decodes a DNS name, following compression pointers. The jump budget
// stops a malicious packet from sending us into an infinite loop.
func readName(msg []byte, offset int) (string, int, error) {
	var parts []string
	jumps := 0
	next := -1

	for {
		if offset >= len(msg) {
			return "", 0, errors.New("name overruns packet")
		}
		length := int(msg[offset])
		switch {
		case length == 0:
			offset++
			if next < 0 {
				next = offset
			}
			return strings.Join(parts, ".") + ".", next, nil
		case length&0xc0 == 0xc0:
			if offset+1 >= len(msg) {
				return "", 0, errors.New("malformed pointer")
			}
			pointer := int(binary.BigEndian.Uint16(msg[offset:offset+2]) & 0x3fff)
			if next < 0 {
				next = offset + 2
			}
			jumps++
			if jumps > 8 || pointer >= len(msg) {
				return "", 0, errors.New("too many pointers")
			}
			offset = pointer
		case length > 63:
			return "", 0, errors.New("invalid label")
		default:
			if offset+1+length > len(msg) {
				return "", 0, errors.New("label overruns packet")
			}
			parts = append(parts, string(msg[offset+1:offset+1+length]))
			offset += 1 + length
		}
	}
}

func writeName(buf []byte, name string) []byte {
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" {
			continue
		}
		if len(label) > 63 {
			label = label[:63]
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	return append(buf, 0)
}

// buildAnswer assembles a response carrying a single A record.
func buildAnswer(id uint16, q question, ip net.IP, ttl uint32, flags uint16, class uint16, echoQuestion bool) []byte {
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}

	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], id)
	binary.BigEndian.PutUint16(header[2:4], flags)
	if echoQuestion {
		binary.BigEndian.PutUint16(header[4:6], 1)
	}
	binary.BigEndian.PutUint16(header[6:8], 1) // one answer

	buf := header
	if echoQuestion {
		buf = writeName(buf, q.name)
		buf = binary.BigEndian.AppendUint16(buf, q.qtype)
		buf = binary.BigEndian.AppendUint16(buf, q.qclass&classMask)
	}

	buf = writeName(buf, q.name)
	buf = binary.BigEndian.AppendUint16(buf, typeA)
	buf = binary.BigEndian.AppendUint16(buf, class)
	buf = binary.BigEndian.AppendUint32(buf, ttl)
	buf = binary.BigEndian.AppendUint16(buf, 4)
	return append(buf, ip4...)
}

// ---------------------------------------------------------------------------
// Address helpers
// ---------------------------------------------------------------------------

// localIPFor picks the address of ours that shares a subnet with the querier,
// so a machine on Wi-Fi is told the Wi-Fi address rather than a Docker bridge.
func localIPFor(remote net.IP) net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return primaryIPv4()
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			if ipnet.Contains(remote) {
				return ipnet.IP.To4()
			}
		}
	}
	return primaryIPv4()
}

// primaryIPv4 returns the address the OS would use to reach the internet,
// which is the best single guess for "the LAN address of this machine".
func primaryIPv4() net.IP {
	conn, err := net.Dial("udp4", "203.0.113.1:9") // TEST-NET-3, no packet is sent
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP.To4() != nil {
			return addr.IP.To4()
		}
	}
	for _, ip := range LocalIPs() {
		return net.ParseIP(ip)
	}
	return nil
}

// LocalIPs lists every non-loopback IPv4 address, used to show the user which
// URLs will work from other devices.
func LocalIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ip4.String())
		}
	}
	return out
}

// URLFor builds a browser URL, hiding the port when it is the default one.
func URLFor(host string, port int) string {
	if port == 80 {
		return fmt.Sprintf("http://%s", host)
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}
