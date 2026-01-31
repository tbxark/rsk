#!/bin/bash
# Multi-Client RSK Setup Example
# This script demonstrates multiple clients with different ports

set -e

# Configuration
TOKEN="secure-random-token-min-16-bytes"
SERVER_PORT=7000

echo "=== RSK Multi-Client Setup Example ==="
echo ""
echo "This example demonstrates:"
echo "1. One RSK server"
echo "2. Three RSK clients on different ports"
echo "3. Testing connections through different exit nodes"
echo ""

# Check if binaries exist
if [ ! -f "./rsk-server" ] || [ ! -f "./rsk-client" ]; then
    echo "Error: rsk-server and rsk-client binaries not found"
    echo "Please build them first:"
    echo "  go build -o rsk-server ./cmd/rsk-server"
    echo "  go build -o rsk-client ./cmd/rsk-client"
    exit 1
fi

# Start server
echo "Starting RSK server on port $SERVER_PORT..."
./rsk-server \
  --listen ":$SERVER_PORT" \
  --token "$TOKEN" \
  --bind 127.0.0.1 \
  --port-range 20000-25000 &
SERVER_PID=$!

sleep 2

# Start first client
echo "Starting Client 1 (port 20001)..."
./rsk-client \
  --server "localhost:$SERVER_PORT" \
  --token "$TOKEN" \
  --ports 20001 \
  --name "client-1" &
CLIENT1_PID=$!

sleep 1

# Start second client
echo "Starting Client 2 (port 20002)..."
./rsk-client \
  --server "localhost:$SERVER_PORT" \
  --token "$TOKEN" \
  --ports 20002 \
  --name "client-2" &
CLIENT2_PID=$!

sleep 1

# Start third client with multiple ports
echo "Starting Client 3 (ports 20003-20005)..."
./rsk-client \
  --server "localhost:$SERVER_PORT" \
  --token "$TOKEN" \
  --ports 20003,20004,20005 \
  --name "client-3-multi-port" &
CLIENT3_PID=$!

sleep 2

# Test connections
echo ""
echo "=== Testing SOCKS5 Connections ==="
echo ""

echo "Testing Client 1 (port 20001):"
curl --socks5 127.0.0.1:20001 -s https://ifconfig.me
echo ""

echo "Testing Client 2 (port 20002):"
curl --socks5 127.0.0.1:20002 -s https://ifconfig.me
echo ""

echo "Testing Client 3 (port 20003):"
curl --socks5 127.0.0.1:20003 -s https://ifconfig.me
echo ""

echo "Testing Client 3 (port 20004):"
curl --socks5 127.0.0.1:20004 -s https://ifconfig.me
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    kill $CLIENT1_PID 2>/dev/null || true
    kill $CLIENT2_PID 2>/dev/null || true
    kill $CLIENT3_PID 2>/dev/null || true
    kill $SERVER_PID 2>/dev/null || true
    echo "Done!"
}

trap cleanup EXIT

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Available SOCKS5 proxies:"
echo "  - Client 1: 127.0.0.1:20001"
echo "  - Client 2: 127.0.0.1:20002"
echo "  - Client 3: 127.0.0.1:20003"
echo "  - Client 3: 127.0.0.1:20004"
echo "  - Client 3: 127.0.0.1:20005"
echo ""
echo "Press Ctrl+C to stop all services."
echo ""

wait
