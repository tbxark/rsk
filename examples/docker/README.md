# Docker Deployment

This directory contains Docker configurations for running RSK server and client in containers.

## Quick Start

### Using Docker Compose

1. **Set your token** (create a `.env` file):
```bash
echo "RSK_TOKEN=$(openssl rand -base64 24)" > .env
```

2. **Start all services**:
```bash
docker-compose up -d
```

3. **View logs**:
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f rsk-server
docker-compose logs -f rsk-client-1
```

4. **Test the connection**:
```bash
# The server exposes ports 20000-20010 to the host
curl --socks5 127.0.0.1:20001 https://ifconfig.me
curl --socks5 127.0.0.1:20002 https://ifconfig.me
```

5. **Stop services**:
```bash
docker-compose down
```

## Building Individual Images

### Server

```bash
# Build from repository root
docker build -f examples/docker/Dockerfile.server -t rsk-server .

# Run server
docker run -d \
  --name rsk-server \
  -p 9527:9527 \
  -p 20000-20010:20000-20010 \
  rsk-server \
  --listen :9527 \
  --token "your-secure-token" \
  --bind 0.0.0.0 \
  --port-range 20000-25000
```

### Client

```bash
# Build from repository root
docker build -f examples/docker/Dockerfile.client -t rsk-client .

# Run client
docker run -d \
  --name rsk-client \
  rsk-client \
  --server your-server.com:9527 \
  --token "your-secure-token" \
  --port 20001 \
  --name "docker-client"
```

## Production Deployment

### Server Configuration

For production, consider:

1. **Use secrets management**:
```yaml
services:
  rsk-server:
    secrets:
      - rsk_token
    command:
      - --token
      - /run/secrets/rsk_token

secrets:
  rsk_token:
    external: true
```

2. **Bind to specific interface**:
```yaml
ports:
  - "127.0.0.1:9527:9527"  # Only localhost
  - "127.0.0.1:20000-20010:20000-20010"
```

3. **Add health checks**:
```yaml
healthcheck:
  test: ["CMD", "nc", "-z", "localhost", "9527"]
  interval: 30s
  timeout: 10s
  retries: 3
```

4. **Resource limits**:
```yaml
deploy:
  resources:
    limits:
      cpus: '1'
      memory: 512M
    reservations:
      cpus: '0.5'
      memory: 256M
```

### Client Configuration

For production clients:

1. **Use environment variables**:
```bash
docker run -d \
  --name rsk-client \
  -e RSK_SERVER=server.example.com:9527 \
  -e RSK_TOKEN_FILE=/run/secrets/token \
  rsk-client
```

2. **Network mode for better performance**:
```yaml
network_mode: host  # Use host networking for better performance
```

3. **Restart policy**:
```yaml
restart: unless-stopped
```

## Multi-Host Deployment

### Server on Host A

```bash
# Start server
docker run -d \
  --name rsk-server \
  --restart unless-stopped \
  -p 0.0.0.0:9527:9527 \
  -p 127.0.0.1:20000-20100:20000-20100 \
  rsk-server \
  --listen :9527 \
  --token "$(cat /run/secrets/rsk-token)" \
  --bind 0.0.0.0 \
  --port-range 20000-30000
```

### Clients on Hosts B, C, D

```bash
# Client on Host B
docker run -d \
  --name rsk-client \
  --restart unless-stopped \
  rsk-client \
  --server server-host-a.example.com:9527 \
  --token "$(cat /run/secrets/rsk-token)" \
  --port 20001 \
  --name "exit-node-b"

# Client on Host C
docker run -d \
  --name rsk-client \
  --restart unless-stopped \
  rsk-client \
  --server server-host-a.example.com:9527 \
  --token "$(cat /run/secrets/rsk-token)" \
  --port 20002 \
  --name "exit-node-c"
```

## Kubernetes Deployment

Example Kubernetes manifests:

### Server Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rsk-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rsk-server
  template:
    metadata:
      labels:
        app: rsk-server
    spec:
      containers:
      - name: rsk-server
        image: rsk-server:latest
        args:
        - --listen
        - ":9527"
        - --token
        - "$(RSK_TOKEN)"
        - --bind
        - "0.0.0.0"
        - --port-range
        - "20000-25000"
        env:
        - name: RSK_TOKEN
          valueFrom:
            secretKeyRef:
              name: rsk-secret
              key: token
        ports:
        - containerPort: 9527
          name: control
        - containerPort: 20000
          name: socks-start
---
apiVersion: v1
kind: Service
metadata:
  name: rsk-server
spec:
  selector:
    app: rsk-server
  ports:
  - port: 9527
    targetPort: 9527
    name: control
  type: LoadBalancer
```

### Client Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rsk-client
spec:
  replicas: 3
  selector:
    matchLabels:
      app: rsk-client
  template:
    metadata:
      labels:
        app: rsk-client
    spec:
      containers:
      - name: rsk-client
        image: rsk-client:latest
        args:
        - --server
        - "rsk-server:9527"
        - --token
        - "$(RSK_TOKEN)"
        - --port
        - "20001"
        - --name
        - "$(POD_NAME)"
        env:
        - name: RSK_TOKEN
          valueFrom:
            secretKeyRef:
              name: rsk-secret
              key: token
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
```

## Troubleshooting

### Container won't start

```bash
# Check logs
docker logs rsk-server
docker logs rsk-client-1

# Check if container is running
docker ps -a

# Inspect container
docker inspect rsk-server
```

### Network connectivity issues

```bash
# Test from client container
docker exec rsk-client-1 ping rsk-server
docker exec rsk-client-1 nc -zv rsk-server 9527

# Check network
docker network inspect rsk-network
```

### Port conflicts

```bash
# Check what's using the port on host
sudo lsof -i :9527
sudo lsof -i :20001

# Use different host ports
docker run -p 7001:9527 ...
```

## Security Considerations

1. **Never expose server port publicly without firewall rules**
2. **Use strong tokens**: `openssl rand -base64 32`
3. **Bind SOCKS5 to localhost only**: `--bind 127.0.0.1`
4. **Use Docker secrets for tokens in production**
5. **Keep images updated**: Rebuild regularly with latest base images
6. **Run as non-root**: Images already configured with non-root user
7. **Use read-only root filesystem** where possible
8. **Limit container resources** to prevent DoS

## Monitoring

### Prometheus Metrics (Future Enhancement)

Consider adding metrics endpoint:
- Active connections
- Bytes transferred
- Connection errors
- Client count

### Log Aggregation

Use Docker logging drivers:
```yaml
logging:
  driver: "syslog"
  options:
    syslog-address: "tcp://logserver:514"
```

Or use log aggregation tools:
- ELK Stack (Elasticsearch, Logstash, Kibana)
- Grafana Loki
- Fluentd
