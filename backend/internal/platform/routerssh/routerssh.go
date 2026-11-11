// Package routerssh reads the router's connection table over SSH (Constitution I.15) and does nothing else: it connects to one private address
// with a key, checks the router's pinned host key, runs one fixed command and returns at most MaxBytes of its output. It sends no other command,
// opens no other channel and forwards nothing. It is the only place the program makes an SSH connection.
package routerssh

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// Command is the one command that is ever run: the router's connection table, read-only.
	Command = "cat /proc/net/nf_conntrack"
	// MaxBytes bounds what is read of the output, and Timeout the whole connection.
	MaxBytes = 4 << 20
	Timeout  = 20 * time.Second
	// DefaultPort is the SSH port when the address names none.
	DefaultPort = 22
)

var (
	// ErrNotPrivate is returned for an address that is not a private IP address on this network: the router is never reached by name or on the internet.
	ErrNotPrivate = errors.New("the router address must be an address on this network, such as 192.168.50.1")
	// ErrHostKeyChanged is returned when the router's host key is not the one that was pinned.
	ErrHostKeyChanged = errors.New("the router's SSH key is not the one you trusted; it may have been reset, or something else is answering")
	// ErrNoHostKey is returned when no host key has been pinned yet.
	ErrNoHostKey = errors.New("trust the router's SSH key first")
	// ErrTooMuch is returned when the output is larger than MaxBytes.
	ErrTooMuch = errors.New("the router's table is larger than this program will read")
	errStop    = errors.New("host key read")
)

// Target is where and as whom to connect. Key is a private key in PEM or OpenSSH form; HostKey is the pinned public key in authorized_keys
// form, empty only when pinning.
type Target struct {
	Address string
	User    string
	Key     []byte
	HostKey string
}

// ParseAddress turns "192.168.50.1" or "192.168.50.1:2222" into an address and port; only a private IP address is accepted.
func ParseAddress(s string) (netip.AddrPort, error) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return permit(ap)
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.AddrPort{}, ErrNotPrivate
	}
	return permit(netip.AddrPortFrom(a, DefaultPort))
}

// permit is the address rule; a test replaces it to reach a server on the loopback.
var permit = checkPrivate

func checkPrivate(ap netip.AddrPort) (netip.AddrPort, error) {
	a := ap.Addr().Unmap()
	if !a.IsPrivate() || a.IsLoopback() {
		return netip.AddrPort{}, ErrNotPrivate
	}
	return netip.AddrPortFrom(a, ap.Port()), nil
}

// Fingerprint is the SHA256 fingerprint of a public key, as ssh-keygen -l shows it, for the operator to compare with the router's own page.
func Fingerprint(key ssh.PublicKey) string { return ssh.FingerprintSHA256(key) }

// ProbeHostKey connects far enough to learn the router's host key and stops before any credential is sent. It returns the key in
// authorized_keys form and its fingerprint.
func ProbeHostKey(ctx context.Context, address string) (line, fingerprint string, err error) {
	var seen ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User: "probe",
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			seen = key
			return errStop
		},
		Timeout: Timeout,
	}
	if _, derr := connect(ctx, address, cfg); seen == nil {
		if derr == nil {
			derr = errors.New("no host key was offered")
		}
		return "", "", derr
	}
	return string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(seen))), Fingerprint(seen), nil
}

// ReadTable connects with the key, checks the host key against the pinned one, runs Command and returns its output.
func ReadTable(ctx context.Context, t Target) ([]byte, error) {
	if t.HostKey == "" {
		return nil, ErrNoHostKey
	}
	pinned, _, _, _, err := ssh.ParseAuthorizedKey([]byte(t.HostKey))
	if err != nil {
		return nil, fmt.Errorf("the pinned router key could not be read: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(t.Key)
	if err != nil {
		return nil, errors.New("the saved key is not a usable private key (it must not be protected by a passphrase)")
	}
	cfg := &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if subtle.ConstantTimeCompare(key.Marshal(), pinned.Marshal()) != 1 {
				return ErrHostKeyChanged
			}
			return nil
		},
		Timeout: Timeout,
	}
	client, err := connect(ctx, t.Address, cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to open a session on the router: %w", err)
	}
	defer session.Close()
	out := &capped{max: MaxBytes}
	session.Stdout = out
	done := make(chan error, 1)
	go func() { done <- session.Run(Command) }()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-done:
		if out.over {
			return nil, ErrTooMuch
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read the router's table: %w", err)
		}
	}
	return out.buf.Bytes(), nil
}

// connect dials one private address under ctx and runs the SSH handshake with cfg.
func connect(ctx context.Context, address string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	ap, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(ap.Addr().String(), strconv.Itoa(int(ap.Port()))))
	if err != nil {
		return nil, fmt.Errorf("failed to reach the router: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(Timeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, ap.String(), cfg)
	if err != nil {
		_ = conn.Close()
		if errors.Is(err, ErrHostKeyChanged) {
			return nil, ErrHostKeyChanged
		}
		return nil, fmt.Errorf("failed to sign in to the router: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

// capped keeps at most max bytes and notes when more came.
type capped struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *capped) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > c.max {
		c.over = true
		return 0, ErrTooMuch
	}
	return c.buf.Write(p)
}
