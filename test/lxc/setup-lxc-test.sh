#!/bin/bash
set -e

# LXC-based VRRP Integration Test Setup
# This provides more realistic testing than Docker

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Configuration
BRIDGE_NAME="vrrp-br0"
BRIDGE_SUBNET="10.10.10.0/24"
BRIDGE_IP="10.10.10.1"
VIP="10.10.10.100"

# Container configuration
CONTAINER1="vrrp-master"
CONTAINER1_IP="10.10.10.10"
CONTAINER1_PRIORITY="200"

CONTAINER2="vrrp-backup"  
CONTAINER2_IP="10.10.10.11"
CONTAINER2_PRIORITY="100"

CONTAINER3="vrrp-client"
CONTAINER3_IP="10.10.10.20"

# Check for LXC/LXD
check_requirements() {
    echo -e "${YELLOW}Checking requirements...${NC}"
    
    # Check for LXC or LXD
    if command -v lxc >/dev/null 2>&1; then
        echo -e "${GREEN}LXD found (snap version)${NC}"
        LXC_CMD="lxc"
    elif command -v lxc-ls >/dev/null 2>&1; then
        echo -e "${GREEN}LXC found (apt version)${NC}"
        LXC_CMD="lxc-"
        echo -e "${YELLOW}Note: This script is optimized for LXD. Consider using Docker tests instead.${NC}"
        echo "To install LXD: sudo snap install lxd && sudo lxd init"
        exit 1
    else
        echo -e "${RED}Neither LXC nor LXD is installed${NC}"
        echo ""
        echo "Option 1 - Install LXD (recommended):"
        echo "  sudo snap install lxd"
        echo "  sudo lxd init --auto"
        echo "  sudo usermod -a -G lxd $USER"
        echo "  newgrp lxd"
        echo ""
        echo "Option 2 - Install LXC (apt):"
        echo "  sudo apt-get update"
        echo "  sudo apt-get install -y lxc lxc-utils bridge-utils"
        echo ""
        echo "Option 3 - Use Docker tests instead (easiest):"
        echo "  make docker-integration-test"
        exit 1
    fi
    
    if [ "$EUID" -ne 0 ] && [ "$LXC_CMD" = "lxc-" ]; then
        echo -e "${YELLOW}Note: LXC commands may need sudo${NC}"
    fi
}

# Create network bridge for LXC
setup_network() {
    echo -e "${GREEN}Setting up LXC network bridge...${NC}"
    
    # Check if network exists
    if lxc network show $BRIDGE_NAME >/dev/null 2>&1; then
        echo "Network $BRIDGE_NAME already exists, deleting..."
        lxc network delete $BRIDGE_NAME --force 2>/dev/null || true
    fi
    
    # Create new bridge network
    lxc network create $BRIDGE_NAME \
        ipv4.address="$BRIDGE_IP/24" \
        ipv4.nat=true \
        ipv6.address=none
    
    echo "Network $BRIDGE_NAME created with subnet $BRIDGE_SUBNET"
}

# Create LXC container with VRRP
create_vrrp_container() {
    local name=$1
    local ip=$2
    local priority=$3
    
    echo -e "${GREEN}Creating container $name...${NC}"
    
    # Delete if exists
    if lxc info $name >/dev/null 2>&1; then
        echo "Container $name exists, deleting..."
        lxc delete --force $name
    fi
    
    # Create container with Ubuntu
    lxc launch ubuntu:22.04 $name
    
    # Wait for container to be ready
    sleep 5
    
    # Attach to our bridge network
    lxc network attach $BRIDGE_NAME $name eth0
    lxc config device set $name eth0 ipv4.address $ip
    
    # Install dependencies in container
    echo "Installing dependencies in $name..."
    lxc exec $name -- apt-get update
    lxc exec $name -- apt-get install -y \
        build-essential \
        golang-go \
        iproute2 \
        iputils-ping \
        tcpdump \
        git
    
    # Copy VRRP source code
    echo "Copying VRRP source to $name..."
    lxc file push -r ../../ $name/root/vrrp-simple/
    
    # Build VRRP in container
    echo "Building VRRP in $name..."
    lxc exec $name -- bash -c "cd /root/vrrp-simple && go build -o /usr/local/bin/vrrp ./main.go"
    
    # Create systemd service for VRRP
    cat << EOF | lxc exec $name -- tee /etc/systemd/system/vrrp.service
[Unit]
Description=VRRP Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/vrrp run \\
    --interface eth0 \\
    --vrid 10 \\
    --priority $priority \\
    --vips $VIP
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    
    # Enable but don't start yet
    lxc exec $name -- systemctl daemon-reload
    lxc exec $name -- systemctl enable vrrp
    
    echo "Container $name configured with priority $priority"
}

# Create client container for testing
create_client_container() {
    local name=$1
    local ip=$2
    
    echo -e "${GREEN}Creating client container $name...${NC}"
    
    # Delete if exists
    if lxc info $name >/dev/null 2>&1; then
        lxc delete --force $name
    fi
    
    # Create container
    lxc launch ubuntu:22.04 $name
    sleep 5
    
    # Configure network
    lxc network attach $BRIDGE_NAME $name eth0
    lxc config device set $name eth0 ipv4.address $ip
    
    # Install tools
    lxc exec $name -- apt-get update
    lxc exec $name -- apt-get install -y iputils-ping tcpdump net-tools
    
    echo "Client container $name ready"
}

# Start VRRP services
start_vrrp() {
    echo -e "${GREEN}Starting VRRP services...${NC}"
    
    lxc exec $CONTAINER1 -- systemctl start vrrp
    echo "Started VRRP on $CONTAINER1 (master)"
    
    sleep 2
    
    lxc exec $CONTAINER2 -- systemctl start vrrp
    echo "Started VRRP on $CONTAINER2 (backup)"
    
    echo "Waiting for VRRP election..."
    sleep 10
}

# Test VIP functionality
test_vip() {
    echo -e "${GREEN}Testing VIP functionality...${NC}"
    
    # Check who has the VIP
    echo -n "Checking VIP ownership... "
    if lxc exec $CONTAINER1 -- ip addr show eth0 | grep -q "$VIP"; then
        echo -e "${GREEN}$CONTAINER1 has VIP${NC}"
        current_master=$CONTAINER1
    elif lxc exec $CONTAINER2 -- ip addr show eth0 | grep -q "$VIP"; then
        echo -e "${GREEN}$CONTAINER2 has VIP${NC}"
        current_master=$CONTAINER2
    else
        echo -e "${RED}No container has VIP!${NC}"
        return 1
    fi
    
    # Test ping from client
    echo "Testing VIP connectivity from client..."
    if lxc exec $CONTAINER3 -- ping -c 3 $VIP; then
        echo -e "${GREEN}✓ VIP is reachable${NC}"
    else
        echo -e "${RED}✗ VIP is not reachable${NC}"
        return 1
    fi
    
    # Show IP configuration
    echo -e "\n${YELLOW}Current IP configuration:${NC}"
    echo "$CONTAINER1 (Priority $CONTAINER1_PRIORITY):"
    lxc exec $CONTAINER1 -- ip addr show eth0 | grep inet
    echo "$CONTAINER2 (Priority $CONTAINER2_PRIORITY):"
    lxc exec $CONTAINER2 -- ip addr show eth0 | grep inet
}

# Test failover
test_failover() {
    echo -e "\n${GREEN}Testing failover...${NC}"
    
    # Stop current master
    if [ "$current_master" = "$CONTAINER1" ]; then
        echo "Stopping VRRP on $CONTAINER1..."
        lxc exec $CONTAINER1 -- systemctl stop vrrp
        expected_new_master=$CONTAINER2
    else
        echo "Stopping VRRP on $CONTAINER2..."
        lxc exec $CONTAINER2 -- systemctl stop vrrp
        expected_new_master=$CONTAINER1
    fi
    
    echo "Waiting for failover (15 seconds)..."
    sleep 15
    
    # Check VIP moved
    echo -n "Checking if VIP moved... "
    if lxc exec $expected_new_master -- ip addr show eth0 | grep -q "$VIP"; then
        echo -e "${GREEN}✓ VIP moved to $expected_new_master${NC}"
    else
        echo -e "${RED}✗ VIP did not move to $expected_new_master${NC}"
        return 1
    fi
    
    # Test connectivity after failover
    echo "Testing VIP connectivity after failover..."
    if lxc exec $CONTAINER3 -- ping -c 3 $VIP; then
        echo -e "${GREEN}✓ VIP still reachable after failover${NC}"
    else
        echo -e "${RED}✗ VIP not reachable after failover${NC}"
        return 1
    fi
}

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    
    # Stop containers
    for container in $CONTAINER1 $CONTAINER2 $CONTAINER3; do
        if lxc info $container >/dev/null 2>&1; then
            echo "Stopping $container..."
            lxc stop --force $container 2>/dev/null || true
            lxc delete --force $container 2>/dev/null || true
        fi
    done
    
    # Remove network
    if lxc network show $BRIDGE_NAME >/dev/null 2>&1; then
        echo "Removing network $BRIDGE_NAME..."
        lxc network delete $BRIDGE_NAME --force 2>/dev/null || true
    fi
    
    echo "Cleanup complete"
}

# Main execution
main() {
    echo -e "${GREEN}=== LXC VRRP Integration Test ===${NC}\n"
    
    # Set trap for cleanup
    trap cleanup EXIT INT TERM
    
    # Run tests
    check_requirements
    setup_network
    create_vrrp_container $CONTAINER1 $CONTAINER1_IP $CONTAINER1_PRIORITY
    create_vrrp_container $CONTAINER2 $CONTAINER2_IP $CONTAINER2_PRIORITY
    create_client_container $CONTAINER3 $CONTAINER3_IP
    start_vrrp
    test_vip
    test_failover
    
    echo -e "\n${GREEN}=== All tests passed! ===${NC}"
    
    # Optional: Keep environment running for manual testing
    read -p "Press Enter to cleanup and exit (or Ctrl+C to keep running)..."
}

# Run if not sourced
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi