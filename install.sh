#!/bin/bash

# seds-agent installation script
# This script installs and configures seds-agent on a remote node

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/opt/seds-agent"
BINARY_NAME="seds-agent"
SERVICE_NAME="seds-agent"
REPO_URL="https://github.com/seds-net/seds-agent"

# Print colored message
print_msg() {
    local color=$1
    shift
    echo -e "${color}$@${NC}"
}

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_msg $RED "Error: This script must be run as root"
        exit 1
    fi
}

# Detect system architecture
detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        armv7l)
            echo "arm"
            ;;
        *)
            print_msg $RED "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
}

# Install sing-box if not present
install_singbox() {
    if command -v sing-box &> /dev/null; then
        print_msg $GREEN "sing-box is already installed"
        sing-box version
        return
    fi

    print_msg $YELLOW "Installing sing-box..."

    # Download and install latest sing-box
    bash <(curl -fsSL https://sing-box.app/gpg.key) -a | sudo gpg --dearmor -o /etc/apt/keyrings/sagernet.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/sagernet.gpg] https://deb.sagernet.org/ * *" | \
        sudo tee /etc/apt/sources.list.d/sagernet.list > /dev/null
    sudo apt-get update
    sudo apt-get install -y sing-box

    print_msg $GREEN "sing-box installed successfully"
}

# Download and install seds-agent
install_agent() {
    local arch=$(detect_arch)
    local binary_url="${REPO_URL}/releases/latest/download/${BINARY_NAME}-linux-${arch}"

    print_msg $YELLOW "Creating installation directory..."
    mkdir -p $INSTALL_DIR

    print_msg $YELLOW "Downloading seds-agent for linux-${arch}..."
    curl -fsSL -o /tmp/${BINARY_NAME} "$binary_url"

    print_msg $YELLOW "Installing binary..."
    mv /tmp/${BINARY_NAME} /usr/local/bin/${BINARY_NAME}
    chmod +x /usr/local/bin/${BINARY_NAME}

    print_msg $GREEN "seds-agent installed to /usr/local/bin/${BINARY_NAME}"
}

# Create configuration file
create_config() {
    print_msg $YELLOW "Creating configuration file..."

    # Prompt for configuration
    read -p "Enter master server address (e.g., master.example.com:2097): " SERVER_ADDR
    read -p "Enter authentication token: " TOKEN

    # Create config file
    cat > $INSTALL_DIR/config.yaml << EOF
server: "${SERVER_ADDR}"
token: "${TOKEN}"
singbox_path: "sing-box"
config_dir: "${INSTALL_DIR}/config"
log_level: "info"
EOF

    chmod 600 $INSTALL_DIR/config.yaml
    print_msg $GREEN "Configuration file created at $INSTALL_DIR/config.yaml"
}

# Create systemd service
create_service() {
    print_msg $YELLOW "Creating systemd service..."

    cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=SEDS Agent
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=/usr/local/bin/${BINARY_NAME} -config ${INSTALL_DIR}/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    print_msg $GREEN "Systemd service created"
}

# Enable and start service
start_service() {
    print_msg $YELLOW "Enabling and starting service..."

    systemctl enable ${SERVICE_NAME}
    systemctl start ${SERVICE_NAME}

    sleep 2

    if systemctl is-active --quiet ${SERVICE_NAME}; then
        print_msg $GREEN "Service started successfully"
        systemctl status ${SERVICE_NAME} --no-pager
    else
        print_msg $RED "Failed to start service"
        systemctl status ${SERVICE_NAME} --no-pager
        exit 1
    fi
}

# Show usage information
show_info() {
    echo ""
    print_msg $GREEN "╔══════════════════════════════════════════════════╗"
    print_msg $GREEN "║         seds-agent installed successfully!       ║"
    print_msg $GREEN "╚══════════════════════════════════════════════════╝"
    echo ""
    echo "Useful commands:"
    echo "  - View logs:        journalctl -u ${SERVICE_NAME} -f"
    echo "  - Check status:     systemctl status ${SERVICE_NAME}"
    echo "  - Restart service:  systemctl restart ${SERVICE_NAME}"
    echo "  - Stop service:     systemctl stop ${SERVICE_NAME}"
    echo "  - Edit config:      nano ${INSTALL_DIR}/config.yaml"
    echo ""
    print_msg $YELLOW "Note: After editing config, restart the service for changes to take effect"
    echo ""
}

# Uninstall function
uninstall() {
    print_msg $YELLOW "Uninstalling seds-agent..."

    # Stop and disable service
    if systemctl is-active --quiet ${SERVICE_NAME}; then
        systemctl stop ${SERVICE_NAME}
    fi
    systemctl disable ${SERVICE_NAME} 2>/dev/null || true
    rm -f /etc/systemd/system/${SERVICE_NAME}.service
    systemctl daemon-reload

    # Remove binary
    rm -f /usr/local/bin/${BINARY_NAME}

    # Remove installation directory
    read -p "Remove configuration directory $INSTALL_DIR? [y/N]: " REMOVE_CONFIG
    if [[ $REMOVE_CONFIG =~ ^[Yy]$ ]]; then
        rm -rf $INSTALL_DIR
        print_msg $GREEN "Configuration directory removed"
    else
        print_msg $YELLOW "Configuration directory preserved at $INSTALL_DIR"
    fi

    print_msg $GREEN "seds-agent uninstalled successfully"
}

# Main installation flow
main() {
    check_root

    echo ""
    print_msg $GREEN "╔══════════════════════════════════════════════════╗"
    print_msg $GREEN "║           seds-agent Installation Script         ║"
    print_msg $GREEN "╚══════════════════════════════════════════════════╝"
    echo ""

    # Check if uninstall flag
    if [ "$1" == "uninstall" ]; then
        uninstall
        exit 0
    fi

    # Install sing-box
    install_singbox

    # Install agent
    install_agent

    # Create configuration
    create_config

    # Create systemd service
    create_service

    # Start service
    start_service

    # Show information
    show_info
}

# Run main function
main "$@"
