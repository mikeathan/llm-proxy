package routerssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// fakeRouter is an SSH server on the loopback that accepts one client key, records every command asked of it, and answers the table.
type fakeRouter struct {
	addr     string
	hostKey  ssh.PublicKey
	mu       sync.Mutex
	commands []string
	output   string
}

func newSigner(t *testing.T) (ssh.Signer, []byte) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return signer, pem.EncodeToMemory(block)
}

func startRouter(t *testing.T, clientKey ssh.PublicKey, output string) *fakeRouter {
	t.Helper()
	old := permit
	permit = func(ap netip.AddrPort) (netip.AddrPort, error) { return ap, nil }
	t.Cleanup(func() { permit = old })
	host, _ := newSigner(t)
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if string(k.Marshal()) == string(clientKey.Marshal()) {
			return nil, nil
		}
		return nil, errors.New("unknown key")
	}}
	cfg.AddHostKey(host)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	r := &fakeRouter{addr: l.Addr().String(), hostKey: host.PublicKey(), output: output}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go r.serve(c, cfg)
		}
	}()
	return r
}

func (r *fakeRouter) serve(c net.Conn, cfg *ssh.ServerConfig) {
	conn, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, _ := nc.Accept()
		go func() {
			for req := range creqs {
				if req.Type != "exec" {
					_ = req.Reply(false, nil)
					continue
				}
				cmd := string(req.Payload[4:])
				r.mu.Lock()
				r.commands = append(r.commands, cmd)
				r.mu.Unlock()
				_ = req.Reply(true, nil)
				_, _ = ch.Write([]byte(r.output))
				_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
				_ = ch.Close()
			}
		}()
	}
}

func TestOnlyAPrivateAddressIsAccepted(t *testing.T) {
	for in, ok := range map[string]bool{"192.168.50.1": true, "192.168.50.1:2222": true, "10.0.0.1": true, "8.8.8.8": false, "127.0.0.1": false, "router.lan": false, "": false, "100.64.0.1": false} {
		_, err := ParseAddress(in)
		if (err == nil) != ok {
			t.Errorf("%q: %v", in, err)
		}
	}
	if ap, _ := ParseAddress("192.168.50.1"); ap.Port() != DefaultPort {
		t.Errorf("port %d", ap.Port())
	}
}

func TestTheHostKeyIsLearntBeforeAnyCredentialIsSentAndTheTableReadWithOneFixedCommand(t *testing.T) {
	signer, pemKey := newSigner(t)
	r := startRouter(t, signer.PublicKey(), "ipv4 2 tcp 6 1 ESTABLISHED src=192.168.1.2 dst=8.8.8.8 sport=1 dport=443\n")
	line, fp, err := ProbeHostKey(context.Background(), r.addr)
	if err != nil || !strings.HasPrefix(fp, "SHA256:") || !strings.HasPrefix(line, "ssh-ed25519 ") {
		t.Fatalf("%q %q %v", line, fp, err)
	}
	out, err := ReadTable(context.Background(), Target{Address: r.addr, User: "admin", Key: pemKey, HostKey: line})
	if err != nil || !strings.Contains(string(out), "dst=8.8.8.8") {
		t.Fatalf("%q %v", out, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.commands) != 1 || !strings.HasSuffix(r.commands[0], Command) {
		t.Errorf("commands run: %q", r.commands)
	}
}

func TestAChangedHostKeyIsRefusedAndNoKeyPinnedMeansNoRead(t *testing.T) {
	signer, pemKey := newSigner(t)
	r := startRouter(t, signer.PublicKey(), "x")
	other, _ := newSigner(t)
	wrong := string(ssh.MarshalAuthorizedKey(other.PublicKey()))
	if _, err := ReadTable(context.Background(), Target{Address: r.addr, User: "admin", Key: pemKey, HostKey: wrong}); !errors.Is(err, ErrHostKeyChanged) {
		t.Errorf("changed key: %v", err)
	}
	if _, err := ReadTable(context.Background(), Target{Address: r.addr, User: "admin", Key: pemKey}); !errors.Is(err, ErrNoHostKey) {
		t.Errorf("no pin: %v", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.commands) != 0 {
		t.Errorf("a command ran: %q", r.commands)
	}
}

func TestAnOversizedTableAndAnUnusableKeyAreRefused(t *testing.T) {
	signer, pemKey := newSigner(t)
	r := startRouter(t, signer.PublicKey(), strings.Repeat("a", MaxBytes+1))
	line, _, _ := ProbeHostKey(context.Background(), r.addr)
	if _, err := ReadTable(context.Background(), Target{Address: r.addr, User: "admin", Key: pemKey, HostKey: line}); !errors.Is(err, ErrTooMuch) {
		t.Errorf("too much: %v", err)
	}
	if _, err := ReadTable(context.Background(), Target{Address: r.addr, User: "admin", Key: []byte("nonsense"), HostKey: line}); err == nil || !strings.Contains(err.Error(), "usable private key") {
		t.Errorf("bad key: %v", err)
	}
}
