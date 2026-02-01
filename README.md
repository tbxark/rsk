# RSK Framework

RSK (Reverse SOCKS over Yamux) is a reverse SOCKS5 proxy system that enables multiple clients to connect to a central server, with each client acting as an exit node for outbound connections. The server exposes SOCKS5 ports locally that applications can use to route traffic through different client exit nodes.

## Features

- **Multi-Client Support**: Multiple clients can connect simultaneously to a single server
- **Port-Based Routing**: Each client claims specific SOCKS5 ports, allowing applications to select exit nodes by port
- **Token Authentication**: Secure token-based authentication prevents unauthorized access
- **Efficient Multiplexing**: Uses yamux to multiplex multiple connections over a single TCP stream
- **Automatic Reconnection**: Clients automatically reconnect on connection failures
- **Clean Resource Management**: Automatic cleanup of resources when clients disconnect

## Architecture

```
┌─────────────┐                    ┌──────────────┐
│ Application │──SOCKS5:20001──────│              │
└─────────────┘                    │              │
                                   │  RSK Server  │──Control TCP──┐
┌─────────────┐                    │  (127.0.0.1) │              │
│ Application │──SOCKS5:20002──────│              │              │
└─────────────┘                    └──────────────┘              │
                                                                 │
                                   ┌──────────────┐              │
                                   │ RSK Client 1 │──────────────┘
                                   │ (Exit Node)  │
                                   └──────────────┘
                                          │
                                          └──► Internet
```

## Installation

### Prerequisites

- Go 1.19 or later

### Building from Source

```bash
# Clone the repository
git clone https://github.com/tbxark/rsk.git
cd rsk

# Build server and client binaries
go build -o rsk-server ./cmd/rsk-server
go build -o rsk-client ./cmd/rsk-client

# Optional: Install to $GOPATH/bin
go install ./cmd/rsk-server
go install ./cmd/rsk-client
```

## Usage

### Server

The RSK server accepts client connections and exposes SOCKS5 ports on localhost.

#### Basic Usage

```bash
./rsk-server --token YOUR_SECRET_TOKEN
```

#### Command-Line Options

| Flag           | Description                                     | Default       | Required |
|----------------|-------------------------------------------------|---------------|----------|
| `--listen`     | Address to listen for client connections        | `:7000`       | No       |
| `--token`      | Authentication token                            | -             | **Yes**  |
| `--bind`       | IP address to bind SOCKS5 listeners             | `127.0.0.1`   | No       |
| `--port-range` | Allowed port range for SOCKS5 (format: min-max) | `20000-40000` | No       |

#### Example

```bash
# Start server on port 7000 with custom port range
./rsk-server \
  --listen :7000 \
  --token "my-secure-token-at-least-16-chars" \
  --bind 127.0.0.1 \
  --port-range 20000-30000
```

### Client

The RSK client connects to the server and handles outbound connections as an exit node.

#### Basic Usage

```bash
./rsk-client \
  --server SERVER_ADDRESS:7000 \
  --token YOUR_SECRET_TOKEN \
  --ports 20001
```

#### Command-Line Options

| Flag             | Description                            | Default  | Required |
|------------------|----------------------------------------|----------|----------|
| `--server`       | Server address (host:port)             | -        | **Yes**  |
| `--token`        | Authentication token                   | -        | **Yes**  |
| `--ports`        | Comma-separated list of ports to claim | -        | **Yes**  |
| `--name`         | Client name for identification         | hostname | No       |
| `--dial-timeout` | Timeout for dialing target addresses   | `15s`    | No       |

#### Example

```bash
# Connect to server and claim port 20001
./rsk-client \
  --server example.com:7000 \
  --token "my-secure-token-at-least-16-chars" \
  --ports 20001 \
  --name "exit-node-us-west"

# Claim multiple ports
./rsk-client \
  --server example.com:7000 \
  --token "my-secure-token-at-least-16-chars" \
  --ports 20001,20002,20003 \
  --name "exit-node-eu-central"
```

## Example Configurations

### Scenario: Multiple Exit Nodes

Set up a server with two clients in different geographic locations:

**1. Start the server:**
```bash
./rsk-server \
  --listen :7000 \
  --token "secure-random-token-min-16-bytes" \
  --port-range 20000-25000
```

**2. Start first client (e.g., US West):**
```bash
./rsk-client \
  --server your-server.com:7000 \
  --token "secure-random-token-min-16-bytes" \
  --ports 20001 \
  --name "us-west-exit"
```

**3. Start second client (e.g., EU Central):**
```bash
./rsk-client \
  --server your-server.com:7000 \
  --token "secure-random-token-min-16-bytes" \
  --ports 20002 \
  --name "eu-central-exit"
```

**4. Use the SOCKS5 proxies:**
```bash
# Route through US West exit node
curl --socks5 127.0.0.1:20001 https://ifconfig.me

# Route through EU Central exit node
curl --socks5 127.0.0.1:20002 https://ifconfig.me
```

### Scenario: Load Balancing with Multiple Ports

One client can claim multiple ports for load balancing:

```bash
# Client claims ports 20001-20004
./rsk-client \
  --server your-server.com:7000 \
  --token "secure-random-token-min-16-bytes" \
  --ports 20001,20002,20003,20004 \
  --name "load-balanced-exit"
```

Applications can then distribute connections across these ports.

## Security Recommendations

### Token Security

1. **Use Strong Tokens**: Generate tokens with at least 16 bytes of randomness
   ```bash
   # Generate a secure token (Linux/macOS)
   openssl rand -base64 24
   ```

2. **Keep Tokens Secret**: Never commit tokens to version control or share them publicly

3. **Rotate Tokens Regularly**: Change tokens periodically, especially if compromise is suspected

### Network Isolation

1. **Bind to Localhost**: The default `--bind 127.0.0.1` ensures SOCKS5 ports are only accessible locally

2. **Firewall Rules**: Restrict access to the server's listening port (default 7000) to authorized clients only

3. **Use TLS/VPN**: Consider running RSK over a VPN or TLS tunnel for additional encryption

### Port Range Configuration

1. **Restrict Port Range**: Use `--port-range` to limit which ports can be allocated
2. **Avoid Privileged Ports**: Don't use ports below 1024
3. **Avoid Conflicts**: Ensure the port range doesn't conflict with other services

## Protocol

RSK uses a custom binary protocol for efficient communication:

### Handshake Protocol

1. **Client → Server: HELLO**
   - Magic: "RSK1" (4 bytes)
   - Version: 0x01 (1 byte)
   - Token length and token (1-255 bytes)
   - Port count and ports (1-16 ports)
   - Client name length and name (0-64 bytes)

2. **Server → Client: HELLO_RESP**
   - Version: 0x01 (1 byte)
   - Status code (1 byte)
   - Accepted ports count and list
   - Optional message

### Connection Protocol

1. **Server → Client: CONNECT_REQ** (per stream)
   - Address length (2 bytes)
   - Target address in "host:port" format

2. **Bidirectional data forwarding** over yamux stream

## Troubleshooting

### Client Cannot Connect

- **Check token**: Ensure client and server use the same token
- **Check network**: Verify the server address is reachable
- **Check firewall**: Ensure port 7000 (or custom listen port) is open

### Port Already in Use

- **Check port conflicts**: Another client may have claimed the port
- **Check system services**: Ensure no other service is using the port
- **Restart server**: Clean up any stale port bindings

### SOCKS5 Connection Fails

- **Check client connection**: Ensure the client is connected to the server
- **Check port mapping**: Verify the SOCKS5 port is claimed by a client
- **Check target address**: Ensure the target is reachable from the client's network

### Performance Issues

- **Network latency**: Check latency between server and clients
- **Client resources**: Ensure clients have sufficient bandwidth and CPU
- **Concurrent connections**: Monitor the number of active streams

## License

See [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please ensure:

1. All tests pass: `go test ./...`
2. Code is formatted: `go fmt ./...`
3. No race conditions: `go test -race ./...`

## Support

For issues, questions, or contributions, please open an issue on the GitHub repository.
