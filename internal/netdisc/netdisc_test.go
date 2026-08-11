package netdisc

import (
	"encoding/binary"
	"net"
	"testing"
)

// buildQuery assembles the kind of packet a real resolver sends, so the parser
// is exercised against the same bytes it will see on the wire.
func buildQuery(id uint16, name string, qtype, qclass uint16) []byte {
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], id)
	binary.BigEndian.PutUint16(header[4:6], 1) // QDCOUNT
	buf := writeName(header, name)
	buf = binary.BigEndian.AppendUint16(buf, qtype)
	return binary.BigEndian.AppendUint16(buf, qclass)
}

func TestParseQuery(t *testing.T) {
	packet := buildQuery(0x1234, "kisaf.local.", typeA, classIN)

	id, flags, questions, err := parseQuery(packet)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0x1234 {
		t.Errorf("id = %#x, wanted 0x1234", id)
	}
	if flags&0x8000 != 0 {
		t.Error("a query packet was marked as a response")
	}
	if len(questions) != 1 {
		t.Fatalf("parsed %d questions, wanted 1", len(questions))
	}
	if questions[0].name != "kisaf.local." {
		t.Errorf("name = %q", questions[0].name)
	}
	if questions[0].qtype != typeA {
		t.Errorf("type = %d", questions[0].qtype)
	}
}

// TestParseQueryRejectsMalformed makes sure a hostile packet from the local
// network cannot panic or hang the responder — it listens on a multicast
// address anyone on the LAN can write to.
func TestParseQueryRejectsMalformed(t *testing.T) {
	cases := map[string][]byte{
		"empty":                 {},
		"half a header":         {0, 1, 2, 3},
		"missing question":      append(make([]byte, 10), 0, 1),
		"pointer loop":          {0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0xc0, 0x0c},
		"overrunning label":     {0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 60, 'a'},
		"absurd question count": {0, 0, 0, 0, 0xff, 0xff, 0, 0, 0, 0, 0, 0},
	}
	for name, packet := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic: %v", name, r)
				}
			}()
			_, _, _, _ = parseQuery(packet)
		}()
	}
}

func TestBuildAnswerMDNS(t *testing.T) {
	q := question{name: "kisaf.local.", qtype: typeA, qclass: classIN}
	msg := buildAnswer(0, q, net.IPv4(192, 168, 1, 42), 120, flagResponseAA, classIN|cacheFlushBit, false)

	if binary.BigEndian.Uint16(msg[2:4]) != flagResponseAA {
		t.Error("wrong response flags")
	}
	if binary.BigEndian.Uint16(msg[4:6]) != 0 {
		t.Error("an mDNS response must not carry a question section")
	}
	if binary.BigEndian.Uint16(msg[6:8]) != 1 {
		t.Error("wanted exactly one answer record")
	}
	// The last four bytes are the address in the A record.
	if got := net.IP(msg[len(msg)-4:]).String(); got != "192.168.1.42" {
		t.Errorf("address = %s", got)
	}
}

func TestBuildAnswerLLMNREchoesQuestion(t *testing.T) {
	q := question{name: "kisaf.", qtype: typeA, qclass: classIN}
	msg := buildAnswer(0xabcd, q, net.IPv4(10, 0, 0, 5), 30, flagLLMNRAnswer, classIN, true)

	if binary.BigEndian.Uint16(msg[0:2]) != 0xabcd {
		t.Error("an LLMNR response must echo the query id")
	}
	if binary.BigEndian.Uint16(msg[4:6]) != 1 {
		t.Error("an LLMNR response must echo the question back")
	}
}

// TestNameRoundTrip checks that what we encode is what we can decode.
func TestNameRoundTrip(t *testing.T) {
	for _, name := range []string{"kisaf.local.", "kisaf.", "a.b.c.d."} {
		encoded := writeName(make([]byte, 0, 64), name)
		// readName expects to start after the header, so prepend 12 bytes.
		packet := append(make([]byte, 12), encoded...)
		got, _, err := readName(packet, 12)
		if err != nil {
			t.Errorf("%q: %v", name, err)
			continue
		}
		if got != name {
			t.Errorf("%q -> %q", name, got)
		}
	}
}

func TestResponderMatches(t *testing.T) {
	r := &Responder{host: "kisaf"}

	if !r.matches(protoMDNS, "kisaf.local.") {
		t.Error("kisaf.local should have matched for mDNS")
	}
	if r.matches(protoMDNS, "kisaf.") {
		t.Error("mDNS must not answer the bare name")
	}
	if !r.matches(protoLLMNR, "KISAF.") {
		t.Error("LLMNR matching must be case insensitive")
	}
	if r.matches(protoLLMNR, "someothermachine.") {
		t.Error("answered for a name that is not ours")
	}
}

func TestURLFor(t *testing.T) {
	if got := URLFor("kisaf.local", 80); got != "http://kisaf.local" {
		t.Errorf("port 80 must not appear in the address: %s", got)
	}
	if got := URLFor("kisaf.local", 7777); got != "http://kisaf.local:7777" {
		t.Errorf("%s", got)
	}
}
