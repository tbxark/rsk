# Systemd Service Files

These systemd service files allow you to run RSK server and client as system services on Linux.

## Installation

### 1. Create RSK User

```bash
sudo useradd -r -s /bin/false rsk
```

### 2. Install Binaries

```bash
# Build binaries
go build -o rsk-server ./cmd/rsk-server
go build -o rsk-client ./cmd/rsk-client

# Install to /opt/rsk
sudo mkdir -p /opt/rsk
sudo cp rsk-server rsk-client /opt/rsk/
sudo chown -R rsk:rsk /opt/rsk
sudo chmod +x /opt/rsk/rsk-server /opt/rsk/rsk-client
```

### 3. Create Log Directory

```bash
sudo mkdir -p /var/log/rsk
sudo chown rsk:rsk /var/log/rsk
```

### 4. Install Service Files

```bash
# Copy service files
sudo cp examples/systemd/rsk-server.service /etc/systemd/system/
sudo cp examples/systemd/rsk-client.service /etc/systemd/system/

# Edit service files to configure your token and settings
sudo nano /etc/systemd/system/rsk-server.service
sudo nano /etc/systemd/system/rsk-client.service

# Reload systemd
sudo systemctl daemon-reload
```

## Configuration

### Server Configuration

Edit `/etc/systemd/system/rsk-server.service` and update:

- `RSK_TOKEN`: Your secure authentication token (minimum 16 bytes)
- `--listen`: Server listening address (default `:9527`)
- `--bind`: SOCKS5 bind address (default `127.0.0.1`)
- `--port-range`: Allowed port range (default `20000-40000`)

### Client Configuration

Edit `/etc/systemd/system/rsk-client.service` and update:

- `RSK_SERVER`: Server address (e.g., `your-server.com:9527`)
- `RSK_TOKEN`: Authentication token (must match server)
- `RSK_PORT`: Port to claim (e.g., `20001`)
- `RSK_NAME`: Client identifier (e.g., `exit-node-us-west`)

## Usage

### Server

```bash
# Start server
sudo systemctl start rsk-server

# Enable auto-start on boot
sudo systemctl enable rsk-server

# Check status
sudo systemctl status rsk-server

# View logs
sudo journalctl -u rsk-server -f

# Stop server
sudo systemctl stop rsk-server
```

### Client

```bash
# Start client
sudo systemctl start rsk-client

# Enable auto-start on boot
sudo systemctl enable rsk-client

# Check status
sudo systemctl status rsk-client

# View logs
sudo journalctl -u rsk-client -f

# Stop client
sudo systemctl stop rsk-client
```

## Multiple Clients

To run multiple clients on the same machine:

```bash
# Copy and rename service file
sudo cp /etc/systemd/system/rsk-client.service /etc/systemd/system/rsk-client-2.service

# Edit the new service file with different configuration
sudo nano /etc/systemd/system/rsk-client-2.service

# Reload and start
sudo systemctl daemon-reload
sudo systemctl start rsk-client-2
sudo systemctl enable rsk-client-2
```

## Security Notes

1. **Never commit service files with real tokens to version control**
2. **Use strong tokens**: Generate with `openssl rand -base64 24`
3. **Restrict file permissions**: `sudo chmod 600 /etc/systemd/system/rsk-*.service`
4. **Use environment files**: Consider using `EnvironmentFile=` for sensitive data
5. **Monitor logs**: Regularly check logs for authentication failures

## Troubleshooting

### Service won't start

```bash
# Check service status
sudo systemctl status rsk-server

# View detailed logs
sudo journalctl -u rsk-server -n 50

# Check configuration
sudo systemctl cat rsk-server
```

### Client can't connect

```bash
# Check network connectivity
ping your-server.com

# Check if server port is open
telnet your-server.com 9527

# Verify token matches
sudo systemctl cat rsk-server | grep RSK_TOKEN
sudo systemctl cat rsk-client | grep RSK_TOKEN
```

### Port conflicts

```bash
# Check what's using a port
sudo lsof -i :20001

# Check RSK server logs
sudo journalctl -u rsk-server | grep PORT_IN_USE
```
