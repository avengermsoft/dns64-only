package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

var (
	debug      bool = false
	r               = &net.Resolver{}
	nat64      string
	nameserver string
	localAddr  string
	localPort  string
)

func isIPv6(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ":")
}

func parseQuery(m *dns.Msg) {
	for _, q := range m.Question {
		switch q.Qtype {
		case dns.TypeAAAA:
			ips, err := r.LookupIP(context.Background(), "ip4", q.Name)
			if debug {
				log.Printf("AAAA query for %s\n", q.Name)
			}
			if err == nil {
				for _, ip := range ips {
					ip_respond := ip.String()
					if strings.Contains(ip_respond, ".") {
						if debug {
							log.Printf("A response for %s: %s\n", q.Name, ip_respond)
						}
						rr, err := dns.NewRR(fmt.Sprintf("%s AAAA %s", q.Name, nat64+ip_respond))
						if err == nil {
							m.Answer = append(m.Answer, rr)
						}
					}
				}
			} else {
				log.Printf("Resolve error for %s: %s\n", q.Name, err)
			}
		}
	}
}

func handleDnsRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	switch r.Opcode {
	case dns.OpcodeQuery:
		parseQuery(m)
	}

	w.WriteMsg(m)
}

func init() {
	flag.StringVar(&localAddr, "localAddr", "127.0.0.1", "Local address to listen on")
	flag.StringVar(&localPort, "localPort", "53", "Local port to listen on")
	flag.StringVar(&nameserver, "dns", "1.1.1.1", "Upstream DNS")
	flag.StringVar(&nat64, "prefix", "64:ff9b::", "NAT64 prefix")
	flag.BoolVar(&debug, "D", false, "Debug mode")
	flag.Parse()
}

func main() {
	lport, err := strconv.ParseUint(localPort, 10, 32)
	if err != nil {
		log.Fatalf("Invalid localPort %s: %s\n ", localPort, err.Error())
	}

	if isIPv6(nameserver) {
		nameserver = "[" + nameserver + "]:53"
	} else {
		nameserver = nameserver + ":53"
	}
	r = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Millisecond * time.Duration(10000),
			}
			return d.DialContext(ctx, network, nameserver)
		},
	}
	log.Printf("Upstream DNS: %s\n", nameserver)
	log.Printf("NAT64 prefix: %s\n", nat64)
	// Attach request handler func
	dns.HandleFunc(".", handleDnsRequest)
	// Start server
	server := &dns.Server{Addr: localAddr + ":" + strconv.Itoa(int(lport)), Net: "udp"}
	log.Printf("DNS64 server starting at %s:%d\n", localAddr, lport)
	err = server.ListenAndServe()
	defer server.Shutdown()
	if err != nil {
		log.Fatalf("Failed to start DNS server: %s\n ", err.Error())
	}
}
