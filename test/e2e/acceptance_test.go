package e2e_test

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/tbxark/rsk/pkg/rsk/client"
	"github.com/tbxark/rsk/pkg/rsk/server"
	"golang.org/x/net/proxy"
)

func TestAcceptance_InvalidTokenRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in -short mode")
	}

	token := []byte("test-token-accept-123")
	serverPort := getFreePort(t)
	socksPort := getFreePort(t)

	srv := startServer(t, &server.Config{
		ListenAddr:        "127.0.0.1:" + itoa(serverPort),
		Token:             token,
		BindIP:            "127.0.0.1",
		PortMin:           1,
		PortMax:           65535,
		MaxClients:        10,
		MaxAuthFailures:   5,
		AuthBlockDuration: time.Second,
		MaxConnsPerClient: 20,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, srv, "server") })

	badClient := startClient(t, &client.Config{
		ServerAddr:           "127.0.0.1:" + itoa(serverPort),
		Token:                []byte("wrong-token-accept-1"),
		Port:                 socksPort,
		Name:                 "bad-token-client",
		DialTimeout:          time.Second,
		AllowPrivateNetworks: true,
	})

	err := waitClientError(t, badClient, 3*time.Second)
	if err == nil {
		t.Fatal("expected invalid token client to fail")
	}

	var hsErr *client.HandshakeError
	if !errors.As(err, &hsErr) || !hsErr.IsAuthFail() {
		t.Fatalf("expected AUTH_FAIL handshake error, got: %v", err)
	}
}

func TestAcceptance_PortConflictRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in -short mode")
	}

	token := []byte("test-token-conflict12")
	serverPort := getFreePort(t)
	socksPort := getFreePort(t)

	srv := startServer(t, &server.Config{
		ListenAddr:        "127.0.0.1:" + itoa(serverPort),
		Token:             token,
		BindIP:            "127.0.0.1",
		PortMin:           1,
		PortMax:           65535,
		MaxClients:        10,
		MaxAuthFailures:   5,
		AuthBlockDuration: time.Second,
		MaxConnsPerClient: 20,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, srv, "server") })

	first := startClient(t, &client.Config{
		ServerAddr:           "127.0.0.1:" + itoa(serverPort),
		Token:                token,
		Port:                 socksPort,
		Name:                 "conflict-client-1",
		DialTimeout:          time.Second,
		AllowPrivateNetworks: true,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, first, "client-1") })

	if err := waitPortOpen("127.0.0.1:"+itoa(socksPort), 3*time.Second); err != nil {
		t.Fatalf("first client socks port not open: %v", err)
	}

	second := startClient(t, &client.Config{
		ServerAddr:           "127.0.0.1:" + itoa(serverPort),
		Token:                token,
		Port:                 socksPort,
		Name:                 "conflict-client-2",
		DialTimeout:          time.Second,
		AllowPrivateNetworks: true,
	})

	err := waitClientError(t, second, 3*time.Second)
	if err == nil {
		t.Fatal("expected conflicting port client to fail")
	}

	var hsErr *client.HandshakeError
	if !errors.As(err, &hsErr) || !hsErr.IsPortInUse() {
		t.Fatalf("expected PORT_IN_USE handshake error, got: %v", err)
	}
}

func TestAcceptance_BlockedTargetFailsSOCKSHandshake(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in -short mode")
	}

	token := []byte("test-token-blocked-12")
	serverPort := getFreePort(t)
	socksPort := getFreePort(t)

	srv := startServer(t, &server.Config{
		ListenAddr:        "127.0.0.1:" + itoa(serverPort),
		Token:             token,
		BindIP:            "127.0.0.1",
		PortMin:           1,
		PortMax:           65535,
		MaxClients:        10,
		MaxAuthFailures:   5,
		AuthBlockDuration: time.Second,
		MaxConnsPerClient: 20,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, srv, "server") })

	cli := startClient(t, &client.Config{
		ServerAddr:           "127.0.0.1:" + itoa(serverPort),
		Token:                token,
		Port:                 socksPort,
		Name:                 "blocked-target-client",
		DialTimeout:          time.Second,
		AllowPrivateNetworks: false,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, cli, "client") })

	socksAddr := "127.0.0.1:" + itoa(socksPort)
	if err := waitPortOpen(socksAddr, 3*time.Second); err != nil {
		t.Fatalf("client socks port not open: %v", err)
	}

	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		t.Fatalf("failed to create SOCKS5 dialer: %v", err)
	}

	conn, err := dialer.Dial("tcp", "127.0.0.1:1")
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected blocked target to fail during SOCKS handshake")
	}
}

func TestAcceptance_ClientIsolationAndCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in -short mode")
	}

	token := []byte("test-token-isolate-12")
	serverPort := getFreePort(t)
	client1Port := getFreePort(t)
	client2Port := getFreePort(t)

	srv := startServer(t, &server.Config{
		ListenAddr:        "127.0.0.1:" + itoa(serverPort),
		Token:             token,
		BindIP:            "127.0.0.1",
		PortMin:           1,
		PortMax:           65535,
		MaxClients:        10,
		MaxAuthFailures:   5,
		AuthBlockDuration: time.Second,
		MaxConnsPerClient: 20,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, srv, "server") })

	client1 := startClient(t, &client.Config{
		ServerAddr:           "127.0.0.1:" + itoa(serverPort),
		Token:                token,
		Port:                 client1Port,
		Name:                 "isolation-client-1",
		DialTimeout:          time.Second,
		AllowPrivateNetworks: true,
	})
	client1Stopped := false
	t.Cleanup(func() {
		if !client1Stopped {
			stopServiceAndAssertClean(t, client1, "client-1")
		}
	})

	client2 := startClient(t, &client.Config{
		ServerAddr:           "127.0.0.1:" + itoa(serverPort),
		Token:                token,
		Port:                 client2Port,
		Name:                 "isolation-client-2",
		DialTimeout:          time.Second,
		AllowPrivateNetworks: true,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, client2, "client-2") })

	socks1 := "127.0.0.1:" + itoa(client1Port)
	socks2 := "127.0.0.1:" + itoa(client2Port)
	if err := waitPortOpen(socks1, 3*time.Second); err != nil {
		t.Fatalf("client1 socks port not open: %v", err)
	}
	if err := waitPortOpen(socks2, 3*time.Second); err != nil {
		t.Fatalf("client2 socks port not open: %v", err)
	}

	upstreamHTTP := startUpstreamHTTPServer(t)
	t.Cleanup(upstreamHTTP.Close)
	upstreamHTTPS := startUpstreamHTTPSServer(t)
	t.Cleanup(upstreamHTTPS.Close)

	httpViaClient1 := newSOCKSHTTPClient(t, socks1, false)
	resp, err := httpViaClient1.Get(upstreamHTTP.URL)
	if err != nil {
		t.Fatalf("http through client1 failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("failed to read http response body: %v", err)
	}
	if string(body) != "upstream-ok" {
		t.Fatalf("unexpected http response body: %q", string(body))
	}

	httpsViaClient2 := newSOCKSHTTPClient(t, socks2, true)
	respTLS, err := httpsViaClient2.Get(upstreamHTTPS.URL)
	if err != nil {
		t.Fatalf("https through client2 failed: %v", err)
	}
	bodyTLS, err := io.ReadAll(respTLS.Body)
	_ = respTLS.Body.Close()
	if err != nil {
		t.Fatalf("failed to read https response body: %v", err)
	}
	if string(bodyTLS) != "upstream-tls-ok" {
		t.Fatalf("unexpected https response body: %q", string(bodyTLS))
	}

	stopServiceAndAssertClean(t, client1, "client-1")
	client1Stopped = true

	if err := waitPortClosed(socks1, 5*time.Second); err != nil {
		t.Logf("client1 socks port cleanup is delayed: %v", err)
	}

	resp2, err := httpsViaClient2.Get(upstreamHTTPS.URL)
	if err != nil {
		t.Fatalf("client2 should remain healthy after client1 stop: %v", err)
	}
	_ = resp2.Body.Close()
}
