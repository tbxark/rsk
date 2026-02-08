package e2e_test

import (
	"io"
	"testing"
	"time"

	"github.com/tbxark/rsk/pkg/rsk/client"
	"github.com/tbxark/rsk/pkg/rsk/server"
)

func TestMultiClientSinglePortEach(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in -short mode")
	}

	token := []byte("test-token-multi-1234")
	serverPort := getFreePort(t)
	ports := []int{getFreePort(t), getFreePort(t), getFreePort(t)}

	srv := startServer(t, &server.Config{
		ListenAddr:        "127.0.0.1:" + itoa(serverPort),
		Token:             token,
		BindIP:            "127.0.0.1",
		PortMin:           1,
		PortMax:           65535,
		MaxClients:        20,
		MaxAuthFailures:   5,
		AuthBlockDuration: time.Second,
		MaxConnsPerClient: 20,
	})
	t.Cleanup(func() { stopServiceAndAssertClean(t, srv, "server") })

	clients := make([]runningService, 0, len(ports))
	for i, p := range ports {
		cli := startClient(t, &client.Config{
			ServerAddr:           "127.0.0.1:" + itoa(serverPort),
			Token:                token,
			Port:                 p,
			Name:                 "multi-client-" + itoa(i+1),
			DialTimeout:          time.Second,
			AllowPrivateNetworks: true,
		})
		clients = append(clients, cli)
	}

	for i := range clients {
		idx := i
		t.Cleanup(func() { stopServiceAndAssertClean(t, clients[idx], "client") })
	}

	upstream := startUpstreamHTTPServer(t)
	t.Cleanup(upstream.Close)

	for _, p := range ports {
		socksAddr := "127.0.0.1:" + itoa(p)
		if err := waitPortOpen(socksAddr, 3*time.Second); err != nil {
			t.Fatalf("socks port not open for %s: %v", socksAddr, err)
		}

		httpClient := newSOCKSHTTPClient(t, socksAddr, false)
		resp, err := httpClient.Get(upstream.URL)
		if err != nil {
			t.Fatalf("request through %s failed: %v", socksAddr, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("failed to read response through %s: %v", socksAddr, err)
		}
		if string(body) != "upstream-ok" {
			t.Fatalf("unexpected response through %s: %q", socksAddr, string(body))
		}
	}
}
