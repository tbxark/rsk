#!/bin/bash
# Basic RSK Setup Example
# This script demonstrates a simple server + client setup

set -e

# Configuration
TOKEN="secure-random-token-min-16-bytes"
SERVER_PORT=7000
CLIENT_PORT=20001

echo "=== RSK Basic Setup Example ==="
echo ""
echo "This example will:"
echo "1. Start an RSK server on port $SERVER_PORT"
echo "2. Start an RSK client claiming port $CLIENT_PORT"
echo "3. Test the SOCKS5 proxy connection"
echo ""

# Check if binaries exist
if [ ! -f "./rsk-server" ] || [ ! -f "./rsk-client" ]; then
    echo "Error: rsk-server and rsk-client binaries not found"
    echo "Please build them first:"
    echo "  go build -o rsk-server ./cmd/rsk-server"
    echo "  go build -o rsk-client ./cmd/rsk-client"
    exit 1
fi

# Start server in background
echo "Starting RSK server..."
./rsk-server \
  --listen ":$SERVER_PORT" \
  --token "$TOKEN" \
  --bind 127.0.0.1 \
  --port-range 20000-25000 &
SERVER_PID=$!

# Wait for server to start
sleep 2

# Start client in background
echo "Starting RSK client..."
./rsk-client \
  --server "localhost:$SERVER_PORT" \
  --token "$TOKEN" \
  --ports "$CLIENT_PORT" \
  --name "example-client" &
CLIENT_PID=$!

# Wait for client to connect
sleep 2

# Test the connection
echo ""
echo "Testing SOCKS5 connection..."
echo "Your exit IP should be:"
curl --socks5 127.0.0.1:$CLIENT_PORT https://ifconfig.me
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    kill $CLIENT_PID 2>/dev/null || true
    kill $SERVER_PID 2>/dev/null || true
    echo "Done!"
}

# Register cleanup on exit
trap cleanup EXIT

echo ""
echo "Setup complete! Press Ctrl+C to stop."
echo ""
echo "You can now use the SOCKS5 proxy at 127.0.0.1:$CLIENT_PORT"
echo "Example: curl --socks5 127.0.0.1:$CLIENT_PORT https://example.com"
echo ""

# Wait for user interrupt
wait
