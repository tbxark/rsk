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
┌─────────────┐                    │  (127.0.0.1) │               │
│ Application │──SOCKS5:20002──────│              │               │
└─────────────┘                    └──────────────┘               │
                                                                  │
                                   ┌──────────────┐               │
                                   │ RSK Client 1 │──────────-────┘
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

| Flag                          | Description                                     | Default       | Required |
|-------------------------------|-------------------------------------------------|---------------|----------|
| `--listen`                    | Address to listen for client connections        | `:7000`       | No       |
| `--token`                     | Authentication token (minimum 16 bytes)         | -             | **Yes**  |
| `--bind`                      | IP address to bind SOCKS5 listeners             | `127.0.0.1`   | No       |
| `--port-range`                | Allowed port range for SOCKS5 (format: min-max) | `20000-40000` | No       |
| `--max-clients`               | Maximum concurrent client connections           | `100`         | No       |
| `--max-auth-failures`         | Failed auth attempts before blocking IP         | `5`           | No       |
| `--auth-block-duration`       | Duration to block IPs after auth failures       | `5m`          | No       |
| `--max-connections-per-client`| Maximum SOCKS5 connections per client           | `100`         | No       |

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

| Flag                      | Description                                    | Default  | Required |
|---------------------------|------------------------------------------------|----------|----------|
| `--server`                | Server address (host:port)                     | -        | **Yes**  |
| `--token`                 | Authentication token (minimum 16 bytes)        | -        | **Yes**  |
| `--ports`                 | Comma-separated list of ports to claim         | -        | **Yes**  |
| `--name`                  | Client name for identification                 | hostname | No       |
| `--dial-timeout`          | Timeout for dialing target addresses           | `15s`    | No       |
| `--allow-private-networks`| Allow connections to private IP ranges         | `false`  | No       |
| `--blocked-networks`      | Additional CIDR blocks to block (comma-separated) | -     | No       |

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

### Scenario: High-Security Deployment

Production deployment with strict security settings:

**Server with enhanced security:**
```bash
./rsk-server \
  --listen :7000 \
  --token "$(cat /etc/rsk/token.secret)" \
  --bind 127.0.0.1 \
  --port-range 20000-20100 \
  --max-clients 50 \
  --max-auth-failures 3 \
  --auth-block-duration 15m \
  --max-connections-per-client 50
```

**Client with network filtering:**
```bash
./rsk-client \
  --server your-server.com:7000 \
  --token "$(cat /etc/rsk/token.secret)" \
  --ports 20001 \
  --name "secure-exit-node" \
  --dial-timeout 10s \
  --blocked-networks "192.0.2.0/24,198.51.100.0/24"
```

**Key security features enabled:**
- Token stored in secure file with restricted permissions
- Limited to 50 concurrent clients
- Aggressive rate limiting (3 failures = 15 minute block)
- Per-client connection limit of 50
- Custom blocked networks on client
- Reduced dial timeout to prevent hanging connections

### Scenario: Development Environment

Relaxed settings for local development:

**Server:**
```bash
./rsk-server \
  --listen :7000 \
  --token "dev-token-16-bytes-min" \
  --max-clients 10 \
  --max-connections-per-client 20
```

**Client with private network access:**
```bash
./rsk-client \
  --server localhost:7000 \
  --token "dev-token-16-bytes-min" \
  --ports 20001 \
  --name "dev-client" \
  --allow-private-networks
```

**Note:** The `--allow-private-networks` flag is useful for development but should be used cautiously in production.

### Scenario: Multi-Region Setup with Monitoring

Enterprise deployment across multiple regions:

**Central Server:**
```bash
./rsk-server \
  --listen :7000 \
  --token "$(openssl rand -base64 32)" \
  --bind 127.0.0.1 \
  --port-range 20000-30000 \
  --max-clients 200 \
  --max-auth-failures 5 \
  --auth-block-duration 10m \
  --max-connections-per-client 100
```

**US East Client:**
```bash
./rsk-client \
  --server central.example.com:7000 \
  --token "$RSK_TOKEN" \
  --ports 20001,20002,20003 \
  --name "us-east-1" \
  --dial-timeout 15s
```

**EU West Client:**
```bash
./rsk-client \
  --server central.example.com:7000 \
  --token "$RSK_TOKEN" \
  --ports 20004,20005,20006 \
  --name "eu-west-1" \
  --dial-timeout 15s
```

**Asia Pacific Client:**
```bash
./rsk-client \
  --server central.example.com:7000 \
  --token "$RSK_TOKEN" \
  --ports 20007,20008,20009 \
  --name "ap-southeast-1" \
  --dial-timeout 15s
```

**Application Usage:**
```bash
# Route through specific region
curl --socks5 127.0.0.1:20001 https://api.example.com  # US East
curl --socks5 127.0.0.1:20004 https://api.example.com  # EU West
curl --socks5 127.0.0.1:20007 https://api.example.com  # Asia Pacific
```

## Security Best Practices

RSK includes multiple security features to protect against common attacks. Follow these best practices to ensure a secure deployment:

### Authentication & Token Management

**Token Strength Requirements**
- RSK enforces a minimum token length of 16 bytes (128 bits)
- Both server and client will refuse to start with tokens shorter than 16 bytes
- Generate cryptographically secure tokens:
  ```bash
  # Linux/macOS - generates 24-byte base64 token
  openssl rand -base64 24
  
  # Alternative: 32-byte hex token
  openssl rand -hex 32
  ```

**Token Storage**
- Store tokens in environment variables or secure configuration files
- Never hardcode tokens in scripts or source code
- Use file permissions to restrict token file access (e.g., `chmod 600`)
- Rotate tokens regularly, especially after personnel changes

**Rate Limiting**
- Server automatically blocks IPs after repeated authentication failures
- Default: 5 failed attempts trigger a 5-minute block
- Adjust thresholds based on your security requirements:
  ```bash
  # More strict: block after 3 failures for 15 minutes
  ./rsk-server --max-auth-failures 3 --auth-block-duration 15m
  ```

### Resource Protection

**Connection Limits**
- Prevent resource exhaustion by limiting concurrent connections
- Server enforces both global and per-client connection limits
- Default limits (100 clients, 100 connections per client) are suitable for most deployments

**Recommended Configurations:**

For small deployments (1-10 clients):
```bash
./rsk-server \
  --max-clients 10 \
  --max-connections-per-client 50
```

For medium deployments (10-50 clients):
```bash
./rsk-server \
  --max-clients 50 \
  --max-connections-per-client 100
```

For large deployments (50+ clients):
```bash
./rsk-server \
  --max-clients 200 \
  --max-connections-per-client 200
```

**Monitoring Connection Usage**
- Monitor server logs for connection limit warnings
- Adjust limits based on actual usage patterns
- Watch for clients hitting per-client limits (may indicate misconfiguration or abuse)

### Network Access Control (Client-Side)

**Address Filtering**
- Clients block dangerous network destinations by default
- Prevents abuse of exit nodes to access internal networks

**Default Blocked Networks:**
- Loopback addresses: `127.0.0.0/8`, `::1`
- Link-local addresses: `169.254.0.0/16`, `fe80::/10`
- Private networks: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`

**Allowing Private Networks (Use with Caution)**
```bash
# Only enable if you need to access private networks
./rsk-client \
  --server example.com:7000 \
  --token "your-secure-token" \
  --ports 20001 \
  --allow-private-networks
```

**Custom Network Blocking**
```bash
# Block additional networks (e.g., corporate networks)
./rsk-client \
  --server example.com:7000 \
  --token "your-secure-token" \
  --ports 20001 \
  --blocked-networks "192.0.2.0/24,198.51.100.0/24"
```

### Deployment Security

**Server Hardening**
1. Run server with minimal privileges (non-root user)
2. Use systemd or similar to manage the service
3. Enable automatic restarts on failure
4. Bind SOCKS5 listeners to localhost only (default)
5. Use firewall rules to restrict server port access

**Client Hardening**
1. Run clients with minimal privileges
2. Enable address filtering (default)
3. Set appropriate dial timeouts to prevent hanging connections
4. Monitor client logs for blocked connection attempts

**Network Security**
1. Deploy server behind a firewall
2. Use VPN or SSH tunnels for additional encryption
3. Consider using TLS reverse proxy (e.g., nginx with TLS termination)
4. Implement network segmentation to isolate RSK traffic

**Monitoring & Alerting**
1. Monitor authentication failure rates
2. Alert on repeated connection limit hits
3. Track blocked network access attempts
4. Review logs regularly for suspicious activity

### Security Checklist

Before deploying to production:

- [ ] Generated strong token (≥16 bytes, cryptographically random)
- [ ] Stored token securely (environment variable or protected file)
- [ ] Configured appropriate connection limits for your scale
- [ ] Enabled rate limiting (using defaults or custom values)
- [ ] Reviewed address filtering settings on clients
- [ ] Configured firewall rules to restrict server access
- [ ] Set up log monitoring and alerting
- [ ] Tested failover and reconnection behavior
- [ ] Documented security configuration for your team
- [ ] Planned token rotation schedule

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

### Server Won't Start

**Token Too Short Error**
```
FATAL: Token too short, minimum 16 bytes required (provided: 8, required: 16)
```
- **Solution**: Generate a stronger token with at least 16 bytes:
  ```bash
  openssl rand -base64 24
  ```

**Port Already in Use**
```
FATAL: Failed to listen on :7000: address already in use
```
- **Solution**: Check if another process is using the port:
  ```bash
  lsof -i :7000
  netstat -tuln | grep 7000
  ```
- Change the listen port: `--listen :7001`

### Client Cannot Connect

**Token Mismatch**
```
ERROR: Authentication failed
```
- **Check token**: Ensure client and server use the exact same token
- **Check token length**: Both must be at least 16 bytes
- Verify no extra whitespace or newlines in token

**IP Blocked Due to Rate Limiting**
```
WARN: IP blocked due to authentication failures ip=X.X.X.X failures=5 block_duration=5m
```
- **Solution**: Wait for the block duration to expire (default: 5 minutes)
- Check if you're using the correct token
- If legitimate, adjust rate limiting: `--max-auth-failures 10`

**Network Unreachable**
- **Check network**: Verify the server address is reachable
  ```bash
  ping your-server.com
  telnet your-server.com 7000
  ```
- **Check firewall**: Ensure port 7000 (or custom listen port) is open
- **Check DNS**: Verify hostname resolves correctly

**Connection Limit Reached**
```
WARN: Connection limit reached, rejecting new connection current=100 max=100
```
- **Solution**: Increase server connection limit: `--max-clients 200`
- Check if there are stale connections
- Restart server to clear any stuck connections

### Port Already in Use

**Port Claimed by Another Client**
```
ERROR: Port 20001 already claimed by another client
```
- **Check port conflicts**: Another client may have claimed the port
- **Solution**: Use a different port or disconnect the other client
- Check server logs to see which client claimed the port

**System Service Using Port**
```
ERROR: Failed to bind SOCKS5 listener on 127.0.0.1:20001
```
- **Check system services**: Ensure no other service is using the port
  ```bash
  lsof -i :20001
  netstat -tuln | grep 20001
  ```
- **Solution**: Choose a different port or stop the conflicting service

### SOCKS5 Connection Fails

**Client Not Connected**
```
ERROR: No client connected on port 20001
```
- **Check client connection**: Ensure the client is connected to the server
- **Check port mapping**: Verify the SOCKS5 port is claimed by a client
- Review server logs for client connection status

**Target Address Blocked**
```
WARN: Target address blocked by filter addr=127.0.0.1:8080 reason="loopback address"
```
- **Solution**: This is expected behavior for security
- If you need to access private networks: `--allow-private-networks`
- For specific networks: `--blocked-networks` to customize filtering

**Per-Client Connection Limit Reached**
```
WARN: Client connection limit reached client_id=XXX port=20001 current=100 max=100
```
- **Solution**: Increase per-client limit: `--max-connections-per-client 200`
- Check if client is leaking connections
- Monitor client connection usage patterns

**Target Unreachable from Client**
```
ERROR: Failed to dial target: connection timeout
```
- **Check target address**: Ensure the target is reachable from the client's network
- **Check client firewall**: Verify outbound connections are allowed
- **Adjust timeout**: Increase dial timeout: `--dial-timeout 30s`

### Performance Issues

**High Latency**
- **Network latency**: Check latency between server and clients
  ```bash
  ping your-server.com
  traceroute your-server.com
  ```
- **Client resources**: Ensure clients have sufficient bandwidth and CPU
- **Connection limits**: Check if hitting connection limits (causes queuing)

**Connection Drops**
- **Network stability**: Check for packet loss or network interruptions
- **Firewall timeouts**: Some firewalls drop idle connections
- **Client reconnection**: Clients automatically reconnect, check logs

**Memory Usage Growing**
- **Check connection counts**: Monitor active connections
- **Rate limiter cleanup**: Runs automatically every minute
- **Restart if needed**: Restart server/client if memory usage is excessive

### Authentication Issues

**Repeated Authentication Failures**
```
WARN: Authentication failed from X.X.X.X (attempt 3/5)
```
- **Check token**: Verify token is correct and at least 16 bytes
- **Check encoding**: Ensure no encoding issues (UTF-8, no BOM)
- **Check logs**: Review server logs for specific error messages

**IP Blocked Unexpectedly**
- **Check rate limit settings**: May be too aggressive
- **Adjust thresholds**: Increase max failures or reduce block duration
  ```bash
  --max-auth-failures 10 --auth-block-duration 2m
  ```
- **Whitelist IPs**: Consider implementing IP whitelisting (future feature)

### Debugging Tips

**Enable Verbose Logging**
- Check server and client logs for detailed error messages
- Look for WARN and ERROR level messages
- Monitor connection and authentication events

**Test Connectivity**
```bash
# Test SOCKS5 proxy is working
curl --socks5 127.0.0.1:20001 https://ifconfig.me

# Test with verbose output
curl -v --socks5 127.0.0.1:20001 https://example.com
```

**Check Resource Usage**
```bash
# Monitor server connections
lsof -i -P | grep rsk-server

# Check memory usage
ps aux | grep rsk

# Monitor network traffic
netstat -an | grep 7000
```

**Common Configuration Mistakes**
1. Token too short (must be ≥16 bytes)
2. Token mismatch between server and client
3. Firewall blocking server port
4. Port conflicts with other services
5. Insufficient connection limits for workload
6. Client trying to access blocked networks without proper flags

## Migration Guide

If you're upgrading from a previous version, please see the [Migration Guide](MIGRATION.md) for important information about:
- Token length requirements (breaking change)
- New security features and default behaviors
- Recommended configurations for different deployment sizes
- Monitoring and troubleshooting tips

## License

See [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please ensure:

1. All tests pass: `go test ./...`
2. Code is formatted: `go fmt ./...`
3. No race conditions: `go test -race ./...`

## Support

For issues, questions, or contributions, please open an issue on the GitHub repository.
