#!/bin/bash
# RSK Test Commands
# Collection of useful commands for testing RSK functionality

echo "=== RSK Test Commands ==="
echo ""
echo "Prerequisites:"
echo "  - RSK server running on localhost:9527"
echo "  - RSK client connected and claiming port 20001"
echo ""

SOCKS_PROXY="127.0.0.1:20001"

echo "1. Check your exit IP address:"
echo "   curl --socks5 $SOCKS_PROXY https://ifconfig.me"
echo ""

echo "2. Test HTTPS connection:"
echo "   curl --socks5 $SOCKS_PROXY https://example.com"
echo ""

echo "3. Test with verbose output:"
echo "   curl -v --socks5 $SOCKS_PROXY https://example.com"
echo ""

echo "4. Download a file through the proxy:"
echo "   curl --socks5 $SOCKS_PROXY -O https://example.com/file.txt"
echo ""

echo "5. Test DNS resolution through SOCKS5:"
echo "   curl --socks5-hostname $SOCKS_PROXY https://example.com"
echo ""

echo "6. Use with wget:"
echo "   https_proxy=socks5h://$SOCKS_PROXY wget https://example.com"
echo ""

echo "7. Use with git:"
echo "   git config --global http.proxy socks5h://$SOCKS_PROXY"
echo "   git clone https://github.com/user/repo.git"
echo "   git config --global --unset http.proxy"
echo ""

echo "8. Use with SSH (ProxyCommand):"
echo "   ssh -o ProxyCommand='nc -X 5 -x $SOCKS_PROXY %h %p' user@remote-host"
echo ""

echo "9. Test IPv6 support:"
echo "   curl --socks5 $SOCKS_PROXY https://ipv6.google.com"
echo ""

echo "10. Benchmark connection speed:"
echo "    curl --socks5 $SOCKS_PROXY -w '@-' -o /dev/null -s https://example.com <<'EOF'"
echo "    time_total: %{time_total}s"
echo "    time_connect: %{time_connect}s"
echo "    time_starttransfer: %{time_starttransfer}s"
echo "    speed_download: %{speed_download} bytes/sec"
echo "EOF"
echo ""

echo "11. Test with multiple concurrent connections:"
echo "    for i in {1..10}; do"
echo "      curl --socks5 $SOCKS_PROXY -s https://ifconfig.me &"
echo "    done"
echo "    wait"
echo ""

echo "12. Test connection timeout handling:"
echo "    curl --socks5 $SOCKS_PROXY --max-time 5 https://example.com"
echo ""

echo "13. Use with Python requests:"
echo "    python3 -c \\"
echo "    import requests; \\"
echo "    proxies = {'http': 'socks5://$SOCKS_PROXY', 'https': 'socks5://$SOCKS_PROXY'}; \\"
echo "    print(requests.get('https://ifconfig.me', proxies=proxies).text)\\"
echo ""

echo "14. Use with Firefox (about:config):"
echo "    network.proxy.type = 1"
echo "    network.proxy.socks = 127.0.0.1"
echo "    network.proxy.socks_port = 20001"
echo "    network.proxy.socks_version = 5"
echo "    network.proxy.socks_remote_dns = true"
echo ""

echo "15. Test error handling (invalid token):"
echo "    ./rsk-client --server localhost:9527 --token wrong-token --port 20002"
echo "    # Should see AUTH_FAIL error"
echo ""

echo "16. Test port conflict detection:"
echo "    # Start first client on port 20001"
echo "    ./rsk-client --server localhost:9527 --token TOKEN --port 20001 &"
echo "    # Try to start second client on same port"
echo "    ./rsk-client --server localhost:9527 --token TOKEN --port 20001"
echo "    # Should see PORT_IN_USE error"
echo ""

echo "17. Monitor server logs:"
echo "    ./rsk-server --token TOKEN 2>&1 | jq -R 'fromjson?'"
echo ""

echo "18. Monitor client logs:"
echo "    ./rsk-client --server localhost:9527 --token TOKEN --port 20001 2>&1 | jq -R 'fromjson?'"
echo ""

echo ""
echo "Run any of these commands to test RSK functionality!"
