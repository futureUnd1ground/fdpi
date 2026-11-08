package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/network"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestNetnsPeer is a real TCP HTTP endpoint in a separate network namespace.
func TestNetnsPeer(t *testing.T) {
	if os.Getenv("FDPI_PEER") != "1" {
		t.Skip("peer helper")
	}
	if _, e := bufio.NewReader(os.Stdin).ReadString('\n'); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", ":80")
	if e != nil {
		t.Fatal(e)
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "discord.com" {
			http.Error(w, "bad host", 400)
			return
		}
		_, _ = io.WriteString(w, strings.Repeat("FDPI-payload-verified\n", 4096))
	})}
	tlsListener, e := net.Listen("tcp", ":443")
	if e != nil {
		t.Fatal(e)
	}
	tlsServer := httptest.NewUnstartedServer(server.Handler)
	_ = tlsServer.Listener.Close()
	tlsServer.Listener = tlsListener
	tlsServer.StartTLS()
	defer tlsServer.Close()
	fmt.Println("PEER_CERT=" + base64.StdEncoding.EncodeToString(tlsServer.Certificate().Raw))
	fmt.Println("PEER_READY")
	if e = server.Serve(listener); e != nil {
		t.Fatal(e)
	}
}
func TestNetnsPayload(t *testing.T) {
	if os.Getenv("FDPI_NETNS_TEST") != "1" {
		t.Skip("isolated netns only")
	}
	self, _ := os.Readlink("/proc/self/ns/net")
	if host := os.Getenv("FDPI_HOST_NETNS"); host == "" || self == host {
		t.Fatal("host namespace refused")
	}
	engine := os.Getenv("FDPI_TEST_ENGINE")
	if engine == "" {
		t.Fatal("real engine required")
	}
	child := exec.Command("unshare", "-n", os.Args[0], "-test.run=^TestNetnsPeer$", "-test.v")
	child.Env = append(os.Environ(), "FDPI_PEER=1")
	child.Stderr = os.Stderr
	stdin, e := child.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	stdout, e := child.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = child.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	run := func(args ...string) {
		t.Helper()
		b, e := exec.Command(args[0], args[1:]...).CombinedOutput()
		if e != nil {
			t.Fatalf("%v: %v %s", args, e, b)
		}
	}
	run("ip", "link", "add", "fdpi-test", "type", "veth", "peer", "name", "fdpi-peer")
	defer exec.Command("ip", "link", "del", "fdpi-test").Run()
	run("ip", "link", "set", "fdpi-peer", "netns", strconv.Itoa(child.Process.Pid))
	run("ip", "addr", "add", "10.203.0.1/24", "dev", "fdpi-test")
	run("ip", "-6", "addr", "add", "fd42:fd91::1/64", "dev", "fdpi-test", "nodad")
	run("ip", "link", "set", "fdpi-test", "up")
	run("ip", "link", "set", "lo", "up")
	peer := func(a ...string) {
		args := append([]string{"nsenter", "-t", strconv.Itoa(child.Process.Pid), "-n", "ip"}, a...)
		run(args...)
	}
	peer("addr", "add", "10.203.0.2/24", "dev", "fdpi-peer")
	peer("-6", "addr", "add", "fd42:fd91::2/64", "dev", "fdpi-peer", "nodad")
	peer("link", "set", "fdpi-peer", "up")
	peer("link", "set", "lo", "up")
	_, _ = io.WriteString(stdin, "start\n")
	_ = stdin.Close()
	ready := make(chan bool, 1)
	var certBytes []byte
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "PEER_CERT=") {
				certBytes, _ = base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "PEER_CERT="))
			}
			if strings.Contains(scanner.Text(), "PEER_READY") {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("peer exited")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("peer timeout")
	}
	cert, e := x509.ParseCertificate(certBytes)
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	if e = os.WriteFile("/proc/sys/net/netfilter/nf_conntrack_acct", []byte("1"), 0600); e != nil {
		t.Fatal(e)
	}
	fetch := func(label string) {
		t.Helper()
		for _, target := range []string{"http://10.203.0.2/", "http://[fd42:fd91::2]/", "https://10.203.0.2/", "https://[fd42:fd91::2]/"} {
			transport := &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}}
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			req, _ := http.NewRequest("GET", target, nil)
			req.Host = "discord.com"
			res, e := client.Do(req)
			if e != nil {
				t.Fatalf("%s %s: %v", label, target, e)
			}
			b, e := io.ReadAll(res.Body)
			_ = res.Body.Close()
			transport.CloseIdleConnections()
			if e != nil || res.StatusCode != 200 || string(b) != strings.Repeat("FDPI-payload-verified\n", 4096) {
				t.Fatalf("%s payload mismatch %v", label, e)
			}
		}
	}
	if e = network.Apply(context.Background(), true, false); e != nil {
		t.Fatal(e)
	}
	defer network.Cleanup(context.Background())
	fetch("queue bypass without listener")
	if e = network.Cleanup(context.Background()); e != nil {
		t.Fatal(e)
	}
	// Model a VPN/PBR route with a smaller MTU for reinjected marked traffic.
	// Generated segments must retain the original veth instead of following this route.
	run("ip", "link", "add", "fdpi-decoy", "type", "dummy")
	defer exec.Command("ip", "link", "del", "fdpi-decoy").Run()
	run("ip", "link", "set", "fdpi-decoy", "mtu", "1376", "up")
	run("ip", "route", "add", "table", "101", "default", "dev", "fdpi-decoy")
	run("ip", "rule", "add", "priority", "101", "fwmark", "0x20000000/0x20000000", "lookup", "101")
	defer exec.Command("ip", "rule", "del", "priority", "101").Run()
	run("ip", "-6", "route", "add", "table", "101", "default", "dev", "fdpi-decoy")
	run("ip", "-6", "rule", "add", "priority", "101", "fwmark", "0x20000000/0x20000000", "lookup", "101")
	defer exec.Command("ip", "-6", "rule", "del", "priority", "101").Run()
	for _, sid := range []string{"light", "balanced", "tls-focus"} {
		c := config.Default()
		c.Profiles[0].Enabled = true
		c.Profiles[0].Strategy = sid
		c.Profiles[0].Domains = []string{"discord.com", "example.com"}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		done := make(chan error, 1)
		dir := t.TempDir()
		go func() { done <- Supervise(ctx, c, engine, dir) }()
		active := false
		for i := 0; i < 200; i++ {
			yes, _ := network.Owned(context.Background())
			if yes {
				active = true
				break
			}
			select {
			case e := <-done:
				cancel()
				t.Fatal(e)
			default:
			}
			time.Sleep(25 * time.Millisecond)
		}
		if !active {
			cancel()
			<-done
			t.Fatal("rules not active")
		}
		iface, e := net.InterfaceByName("fdpi-test")
		if e != nil {
			cancel()
			<-done
			t.Fatal(e)
		}
		fd, e := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_NONBLOCK, int((unix.ETH_P_ALL<<8)&0xff00|unix.ETH_P_ALL>>8))
		if e != nil {
			cancel()
			<-done
			t.Fatal(e)
		}
		if e = unix.Bind(fd, &unix.SockaddrLinklayer{Protocol: uint16((unix.ETH_P_ALL<<8)&0xff00 | unix.ETH_P_ALL>>8), Ifindex: iface.Index}); e != nil {
			unix.Close(fd)
			cancel()
			<-done
			t.Fatal(e)
		}
		// Keep only outgoing IPv4 TCP requests so bulk responses cannot exhaust capture buffers.
		filter := []unix.SockFilter{
			{Code: unix.BPF_LD | unix.BPF_H | unix.BPF_ABS, K: 12},
			{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: 0x0800, Jf: 6},
			{Code: unix.BPF_LD | unix.BPF_B | unix.BPF_ABS, K: 23},
			{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: 6, Jf: 4},
			{Code: unix.BPF_LDX | unix.BPF_B | unix.BPF_MSH, K: 14},
			{Code: unix.BPF_LD | unix.BPF_H | unix.BPF_IND, K: 16},
			{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: 80, Jt: 2},
			{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: 443, Jt: 1},
			{Code: unix.BPF_RET | unix.BPF_K, K: 0},
			{Code: unix.BPF_RET | unix.BPF_K, K: 65535},
		}
		if e = unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}); e != nil {
			unix.Close(fd)
			cancel()
			<-done
			t.Fatal(e)
		}
		fetch(sid)
		lengths := map[int]int{}
		tlsLengths := map[int]int{}
		packet := make([]byte, 65536)
		for i := 0; i < 2048; i++ {
			n, _, re := unix.Recvfrom(fd, packet, 0)
			if re != nil {
				break
			}
			if n < 54 || binary.BigEndian.Uint16(packet[12:14]) != 0x0800 {
				continue
			}
			ihl := int(packet[14]&15) * 4
			if n < 14+ihl+20 || packet[23] != 6 {
				continue
			}
			tcpStart := 14 + ihl
			port := binary.BigEndian.Uint16(packet[tcpStart+2 : tcpStart+4])
			if port != 80 && port != 443 {
				continue
			}
			payload := int(binary.BigEndian.Uint16(packet[16:18])) - ihl - int(packet[tcpStart+12]>>4)*4
			if payload > 0 {
				if port == 80 {
					lengths[payload]++
				} else {
					tlsLengths[payload]++
				}
			}
		}
		unix.Close(fd)
		if sid == "light" && lengths[1] == 0 {
			cancel()
			<-done
			t.Fatalf("no actual byte-1 split on wire: %v", lengths)
		}
		t.Logf("%s observed IPv4 TCP payload lengths on veth: HTTP=%v TLS=%v", sid, lengths, tlsLengths)
		if len(tlsLengths) == 0 {
			cancel()
			<-done
			t.Fatal("no TLS packets captured")
		}
		b, e := exec.Command("/usr/bin/nft", "list", "table", "inet", "fdpi").CombinedOutput()
		if e != nil || strings.Contains(string(b), "counter packets 0 bytes 0") {
			cancel()
			<-done
			t.Fatalf("no queued traffic: %v %s", e, b)
		}
		t.Logf("%s IPv4+IPv6 exact HTTP+HTTPS payload delivered; %s", sid, strings.TrimSpace(string(b)))
		cancel()
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	}
}
