package e2e_test

import (
	"io"
	"testing"
	"time"

	"github.com/tbxark/rsk/pkg/rsk/client"
	"github.com/tbxark/rsk/pkg/rsk/server"
)

func TestBasicSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in -short mode")
	}

	token := []byte("test-token-basic-12345")
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
		MaxConnsPerClient: 50,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, srv, "server") })

	cli := startClient(t, &client.Config{
		ServerAddr:           "127.0.0.1:" + itoa(serverPort),
		Token:                token,
		Port:                 socksPort,
		Name:                 "basic-client",
		DialTimeout:          time.Second,
		AllowPrivateNetworks: true,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, cli, "client") })

	socksAddr := "127.0.0.1:" + itoa(socksPort)
	if err := waitPortOpen(socksAddr, 3*time.Second); err != nil {
		t.Fatalf("socks port did not open: %v", err)
	}

	upstream := startUpstreamHTTPServer(t)
	t.Cleanup(upstream.Close)

	httpClient := newSOCKSHTTPClient(t, socksAddr, false)
	resp, err := httpClient.Get(upstream.URL)
	if err != nil {
		t.Fatalf("request through socks failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if string(body) != "upstream-ok" {
		t.Fatalf("unexpected response body: %q", string(body))
	}
}
