# SEDS Agent

> **Notice:** This project is the agent component for [seds-server](https://github.com/seds-net/seds-server) (forked from s-ui) and is currently **under active development**. Use at your own risk.

Lightweight agent that runs on remote nodes, connecting to the SEDS Server via gRPC to manage the local sing-box process.

## Features

- gRPC bidirectional streaming with auto-reconnection
- Sing-box subprocess lifecycle management
- System monitoring (CPU, memory, disk, network)
- Remote configuration sync and command execution

## Quick Start

```sh
# Build
make proto && make build

# Generate config
./build/seds-agent -gen-config

# Edit config.yaml with your server address and token, then run
./build/seds-agent -config config.yaml
```

## Configuration

```yaml
server: "master.example.com:2097"
token: "your-token-from-server"
singbox_path: "sing-box"
config_dir: "./config"
log_level: "info"
```

All settings can be overridden via flags: `-server`, `-token`, `-singbox`, `-config`.

## Systemd Service

```ini
[Unit]
Description=SEDS Agent
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/seds-agent
ExecStart=/usr/local/bin/seds-agent -config /opt/seds-agent/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl enable seds-agent --now
sudo journalctl -u seds-agent -f
```

## Automated Install

```sh
bash <(curl -fsSL https://raw.githubusercontent.com/seds-net/seds-agent/main/install.sh)
```

## License

GPL-3.0
