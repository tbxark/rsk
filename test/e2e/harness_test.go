package e2e_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tbxark/rsk/pkg/rsk/client"
	"github.com/tbxark/rsk/pkg/rsk/server"
	"golang.org/x/net/proxy"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func getFreePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer func() { _ = l.Close() }()

	return l.Addr().(*net.TCPAddr).Port
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func getNonLoopbackIPv4(t *testing.T) string {
	t.Helper()

	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("failed to list interfaces: %v", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			return ip.String()
		}
	}

	t.Skip("no non-loopback IPv4 found for e2e upstream server")
	return ""
}

func waitPortOpen(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("port did not open in time: %s", addr)
}

func waitPortClosed(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			_ = listener.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("port did not close in time: %s", addr)
}

type runningService struct {
	cancel context.CancelFunc
	errCh  <-chan error
}

func startServer(t *testing.T, cfg *server.Config) runningService {
	t.Helper()

	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	srv := server.NewServer(cfg, testLogger())

	go func() {
		err := srv.Start(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
		close(errCh)
	}()

	if err := waitPortOpen(cfg.ListenAddr, 3*time.Second); err != nil {
		cancel()
		t.Fatalf("server failed to start: %v", err)
	}

	return runningService{cancel: cancel, errCh: errCh}
}

func startClient(t *testing.T, cfg *client.Config) runningService {
	t.Helper()

	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cli := &client.Client{
		Config:         cfg,
		ReconnectDelay: 50 * time.Millisecond,
		Logger:         testLogger(),
	}

	go func() {
		err := cli.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
		close(errCh)
	}()

	return runningService{cancel: cancel, errCh: errCh}
}

func stopServiceAndAssertClean(t *testing.T, svc runningService, name string) {
	t.Helper()

	svc.cancel()
	select {
	case err, ok := <-svc.errCh:
		if ok && err != nil {
			t.Fatalf("%s exited with unexpected error: %v", name, err)
		}
	case <-time.After(3 * time.Second):
		t.Logf("timed out waiting %s to stop; continuing cleanup", name)
	}
}

func waitClientError(t *testing.T, svc runningService, timeout time.Duration) error {
	t.Helper()

	select {
	case err, ok := <-svc.errCh:
		if !ok {
			return nil
		}
		return err
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for client error")
		return nil
	}
}

func startUpstreamHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("upstream-ok"))
	}))
	listener, err := net.Listen("tcp", net.JoinHostPort(getNonLoopbackIPv4(t), "0"))
	if err != nil {
		t.Fatalf("failed to listen for upstream http server: %v", err)
	}
	srv.Listener = listener
	srv.Start()
	return srv
}

func startUpstreamHTTPSServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("upstream-tls-ok"))
	}))
	listener, err := net.Listen("tcp", net.JoinHostPort(getNonLoopbackIPv4(t), "0"))
	if err != nil {
		t.Fatalf("failed to listen for upstream https server: %v", err)
	}
	srv.Listener = listener
	srv.StartTLS()
	return srv
}

func newSOCKSHTTPClient(t *testing.T, socksAddr string, insecureTLS bool) *http.Client {
	t.Helper()

	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		t.Fatalf("failed to create SOCKS5 dialer: %v", err)
	}

	transport := &http.Transport{
		DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureTLS}, //nolint:gosec
	}

	return &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
}
