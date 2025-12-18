#!/bin/bash

# Autohost Agent Installation Script
# This script installs the autohost-agent as a systemd service

set -e

BINARY_NAME="autohost-agent"
INSTALL_PATH="/usr/local/bin"
CONFIG_PATH="/etc/autohost"
SERVICE_PATH="/etc/systemd/system"

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "Please run as root or with sudo"
    exit 1
fi

echo "Installing Autohost Agent..."

# Create directories
mkdir -p "$CONFIG_PATH"

# Copy binary
if [ -f "$BINARY_NAME" ]; then
    cp "$BINARY_NAME" "$INSTALL_PATH/"
    chmod +x "$INSTALL_PATH/$BINARY_NAME"
    echo "✓ Binary installed to $INSTALL_PATH/$BINARY_NAME"
else
    echo "Error: Binary not found. Please run 'make build' first."
    exit 1
fi

# Copy service file
if [ -f "autohost-agent.service" ]; then
    cp autohost-agent.service "$SERVICE_PATH/"
    echo "✓ Service file installed"
else
    echo "Warning: Service file not found"
fi

# Copy config if it doesn't exist
if [ ! -f "$CONFIG_PATH/config.yaml" ]; then
    if [ -f "configs/agent.yaml" ]; then
        cp configs/agent.yaml "$CONFIG_PATH/config.yaml"
        chmod 600 "$CONFIG_PATH/config.yaml"
        echo "✓ Config file created at $CONFIG_PATH/config.yaml"
        echo ""
        echo "IMPORTANT: Edit $CONFIG_PATH/config.yaml with your settings before starting the agent"
    else
        echo "Warning: Example config not found"
    fi
else
    echo "✓ Config file already exists at $CONFIG_PATH/config.yaml"
fi

# Reload systemd
systemctl daemon-reload
echo "✓ Systemd reloaded"

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Edit the configuration: sudo nano $CONFIG_PATH/config.yaml"
echo "  2. Enable the service: sudo systemctl enable $BINARY_NAME"
echo "  3. Start the service: sudo systemctl start $BINARY_NAME"
echo "  4. Check status: sudo systemctl status $BINARY_NAME"
echo ""
