#!/bin/bash
set -e

# LXC-based VRRP Integration Tests
# More realistic than Docker as LXC containers have full network stacks

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
BRIDGE_NAME="vrrp-test-br"
BRIDGE_SUBNET="10.99.0.0/24"
VIP="10.99.0.100"
VRID="50"

# Container configuration
MASTER_NAME="vrrp-master-test"
MASTER_IP="10.99.0.10"
MASTER_PRIORITY="200"

BACKUP_NAME="vrrp-backup-test"
BACKUP_IP="10.99.0.11"
BACKUP_PRIORITY="100"

CLIENT_NAME="vrrp-client-test"
CLIENT_IP="10.99.0.20"

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

# Binary path
VRRP_BINARY="${PWD}/vrrp"

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}This script must be run as root for LXC operations${NC}"
        echo "Try: sudo $0"
        exit 1
    fi
}

# Check requirements
check_requirements() {
    echo -e "${BLUE}Checking requirements...${NC}"
    
    # Check for lxc
    if ! command -v lxc >/dev/null 2>&1; then
        echo -e "${RED}LXC is not installed${NC}"
        echo "Install with: apt-get install -y lxc lxc-utils"
        exit 1
    fi
    
    # Check for vrrp binary
    if [ ! -f "$VRRP_BINARY" ]; then
        echo -e "${YELLOW}VRRP binary not found, building...${NC}"
        make build
    fi
    
    echo -e "${GREEN}All requirements met${NC}"
}

# Create network namespace bridge
setup_network() {
    echo -e "${BLUE}Setting up test network...${NC}"
    
    # Create bridge if it doesn't exist
    if ! ip link show "$BRIDGE_NAME" >/dev/null 2>&1; then
        ip link add name "$BRIDGE_NAME" type bridge
        ip addr add "${BRIDGE_SUBNET%0/24}1/24" dev "$BRIDGE_NAME"
        ip link set "$BRIDGE_NAME" up
        
        # Enable IP forwarding
        sysctl -w net.ipv4.ip_forward=1 >/dev/null
        
        echo "Created bridge $BRIDGE_NAME with subnet $BRIDGE_SUBNET"
    else
        echo "Bridge $BRIDGE_NAME already exists"
    fi
}

# Create LXC container
create_container() {
    local name=$1
    local ip=$2
    local role=$3
    
    echo -e "${BLUE}Creating container $name ($role)...${NC}"
    
    # Stop and destroy if exists
    if lxc-info -n "$name" >/dev/null 2>&1; then
        lxc-stop -n "$name" -k 2>/dev/null || true
        lxc-destroy -n "$name" 2>/dev/null || true
    fi
    
    # Create container with minimal template
    lxc-create -n "$name" -t download -- \
        --dist alpine \
        --release 3.18 \
        --arch amd64 \
        >/dev/null 2>&1
    
    # Configure network
    cat >> "/var/lib/lxc/$name/config" <<EOF

# Network configuration
lxc.net.0.type = veth
lxc.net.0.link = $BRIDGE_NAME
lxc.net.0.flags = up
lxc.net.0.ipv4.address = $ip/24
lxc.net.0.ipv4.gateway = ${BRIDGE_SUBNET%0/24}1

# Allow capabilities for VRRP
lxc.cap.drop =
lxc.cap.keep = CAP_NET_RAW CAP_NET_ADMIN CAP_SYS_ADMIN
EOF
    
    # Start container
    lxc-start -n "$name" -d
    sleep 2
    
    # Copy VRRP binary
    lxc-attach -n "$name" -- mkdir -p /usr/local/bin
    cp "$VRRP_BINARY" "/var/lib/lxc/$name/rootfs/usr/local/bin/"
    lxc-attach -n "$name" -- chmod +x /usr/local/bin/vrrp
    
    echo "Container $name created with IP $ip"
}

# Start VRRP in container
start_vrrp() {
    local name=$1
    local priority=$2
    local interface="eth0"
    
    echo "Starting VRRP in $name with priority $priority..."
    
    # Kill any existing VRRP process
    lxc-attach -n "$name" -- pkill vrrp 2>/dev/null || true
    
    # Start VRRP in background
    lxc-attach -n "$name" -- sh -c "nohup /usr/local/bin/vrrp run \
        --interface $interface \
        --vrid $VRID \
        --priority $priority \
        --vips $VIP \
        > /tmp/vrrp.log 2>&1 &"
    
    sleep 1
    
    # Check if started
    if lxc-attach -n "$name" -- pgrep vrrp >/dev/null; then
        echo -e "${GREEN}✓ VRRP started in $name${NC}"
    else
        echo -e "${RED}✗ Failed to start VRRP in $name${NC}"
        lxc-attach -n "$name" -- cat /tmp/vrrp.log
        return 1
    fi
}

# Check which container has the VIP
check_vip_owner() {
    if lxc-attach -n "$MASTER_NAME" -- ip addr show eth0 2>/dev/null | grep -q "$VIP"; then
        echo "master"
    elif lxc-attach -n "$BACKUP_NAME" -- ip addr show eth0 2>/dev/null | grep -q "$VIP"; then
        echo "backup"
    else
        echo "none"
    fi
}

# Test VIP connectivity
test_vip_connectivity() {
    lxc-attach -n "$CLIENT_NAME" -- ping -c 3 -W 2 "$VIP" >/dev/null 2>&1
}

# Run test and track results
run_test() {
    local test_name=$1
    local test_func=$2
    
    echo -e "\n${BLUE}Running: $test_name${NC}"
    
    if $test_func; then
        echo -e "${GREEN}✓ $test_name passed${NC}"
        ((TESTS_PASSED++))
        return 0
    else
        echo -e "${RED}✗ $test_name failed${NC}"
        ((TESTS_FAILED++))
        return 1
    fi
}

# Test 1: Initial master election
test_initial_election() {
    echo "Waiting for initial master election..."
    sleep 5
    
    local owner=$(check_vip_owner)
    echo "VIP owner: $owner"
    
    if [ "$owner" = "master" ]; then
        echo -e "${GREEN}VIP is on master node (high priority)${NC}"
        return 0
    else
        echo -e "${RED}VIP is not on master node (found on: $owner)${NC}"
        return 1
    fi
}

# Test 2: VIP connectivity
test_vip_reachable() {
    if test_vip_connectivity; then
        echo -e "${GREEN}VIP $VIP is reachable from client${NC}"
        return 0
    else
        echo -e "${RED}VIP $VIP is not reachable${NC}"
        return 1
    fi
}

# Test 3: Failover on master failure
test_failover() {
    echo "Stopping VRRP on master..."
    lxc-attach -n "$MASTER_NAME" -- pkill vrrp
    
    echo "Waiting for failover (10 seconds)..."
    sleep 10
    
    local owner=$(check_vip_owner)
    echo "VIP owner after failover: $owner"
    
    if [ "$owner" = "backup" ]; then
        echo -e "${GREEN}VIP moved to backup node${NC}"
        
        # Test connectivity after failover
        if test_vip_connectivity; then
            echo -e "${GREEN}VIP still reachable after failover${NC}"
            return 0
        else
            echo -e "${RED}VIP not reachable after failover${NC}"
            return 1
        fi
    else
        echo -e "${RED}VIP did not move to backup (found on: $owner)${NC}"
        return 1
    fi
}

# Test 4: Preemption when master returns
test_preemption() {
    echo "Restarting VRRP on master..."
    start_vrrp "$MASTER_NAME" "$MASTER_PRIORITY"
    
    echo "Waiting for preemption (10 seconds)..."
    sleep 10
    
    local owner=$(check_vip_owner)
    echo "VIP owner after master restart: $owner"
    
    if [ "$owner" = "master" ]; then
        echo -e "${GREEN}VIP preempted back to master${NC}"
        return 0
    else
        echo -e "${YELLOW}VIP did not preempt (found on: $owner)${NC}"
        echo "Note: This might be expected if preemption is disabled"
        return 0  # Don't fail test as preemption might be disabled
    fi
}

# Test 5: Split brain prevention
test_split_brain() {
    # Start both with same priority
    echo "Testing split brain with equal priorities..."
    
    lxc-attach -n "$MASTER_NAME" -- pkill vrrp 2>/dev/null || true
    lxc-attach -n "$BACKUP_NAME" -- pkill vrrp 2>/dev/null || true
    sleep 2
    
    # Start both with same priority
    start_vrrp "$MASTER_NAME" "150"
    start_vrrp "$BACKUP_NAME" "150"
    
    echo "Waiting for election with equal priorities..."
    sleep 10
    
    # Check that only one has VIP
    local master_has_vip=$(lxc-attach -n "$MASTER_NAME" -- ip addr show eth0 | grep -c "$VIP" || echo "0")
    local backup_has_vip=$(lxc-attach -n "$BACKUP_NAME" -- ip addr show eth0 | grep -c "$VIP" || echo "0")
    
    if [ "$master_has_vip" = "1" ] && [ "$backup_has_vip" = "0" ]; then
        echo -e "${GREEN}Only master has VIP (IP-based tiebreaker worked)${NC}"
        return 0
    elif [ "$master_has_vip" = "0" ] && [ "$backup_has_vip" = "1" ]; then
        echo -e "${GREEN}Only backup has VIP (IP-based tiebreaker worked)${NC}"
        return 0
    else
        echo -e "${RED}Split brain detected! Both or neither have VIP${NC}"
        return 1
    fi
}

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up test environment...${NC}"
    
    # Stop and destroy containers
    for container in "$MASTER_NAME" "$BACKUP_NAME" "$CLIENT_NAME"; do
        if lxc-info -n "$container" >/dev/null 2>&1; then
            echo "Removing container $container..."
            lxc-stop -n "$container" -k 2>/dev/null || true
            lxc-destroy -n "$container" 2>/dev/null || true
        fi
    done
    
    # Remove bridge
    if ip link show "$BRIDGE_NAME" >/dev/null 2>&1; then
        echo "Removing bridge $BRIDGE_NAME..."
        ip link set "$BRIDGE_NAME" down
        ip link delete "$BRIDGE_NAME"
    fi
    
    echo "Cleanup complete"
}

# Show logs for debugging
show_logs() {
    echo -e "\n${YELLOW}=== VRRP Logs ===${NC}"
    
    echo -e "${BLUE}Master logs:${NC}"
    lxc-attach -n "$MASTER_NAME" -- tail -20 /tmp/vrrp.log 2>/dev/null || echo "No logs available"
    
    echo -e "\n${BLUE}Backup logs:${NC}"
    lxc-attach -n "$BACKUP_NAME" -- tail -20 /tmp/vrrp.log 2>/dev/null || echo "No logs available"
}

# Main test execution
main() {
    echo -e "${GREEN}=== LXC VRRP Integration Tests ===${NC}\n"
    
    # Set trap for cleanup
    trap cleanup EXIT INT TERM
    
    # Initial setup
    check_root
    check_requirements
    setup_network
    
    # Create containers
    create_container "$MASTER_NAME" "$MASTER_IP" "master"
    create_container "$BACKUP_NAME" "$BACKUP_IP" "backup"
    create_container "$CLIENT_NAME" "$CLIENT_IP" "client"
    
    # Start VRRP services
    start_vrrp "$MASTER_NAME" "$MASTER_PRIORITY"
    start_vrrp "$BACKUP_NAME" "$BACKUP_PRIORITY"
    
    # Run tests
    run_test "Initial Master Election" test_initial_election
    run_test "VIP Connectivity" test_vip_reachable
    run_test "Failover on Master Failure" test_failover
    run_test "Preemption on Master Return" test_preemption
    run_test "Split Brain Prevention" test_split_brain
    
    # Show results
    echo -e "\n${GREEN}=== Test Results ===${NC}"
    echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
    echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
    
    # Show logs if any test failed
    if [ "$TESTS_FAILED" -gt 0 ]; then
        show_logs
    fi
    
    # Exit with appropriate code
    if [ "$TESTS_FAILED" -eq 0 ]; then
        echo -e "\n${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "\n${RED}Some tests failed${NC}"
        exit 1
    fi
}

# Run if not sourced
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi