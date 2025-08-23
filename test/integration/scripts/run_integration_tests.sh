#!/bin/bash
set -e

# Main integration test runner for VRRP
# Must be run as root for namespace operations

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
TEST_DIR="$PROJECT_ROOT/test/integration"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo -e "${RED}This script must be run as root for network namespace operations${NC}"
    exit 1
fi

echo -e "${GREEN}Starting VRRP Integration Tests${NC}"
echo "Project root: $PROJECT_ROOT"

# Build the VRRP binary
echo -e "${YELLOW}Building VRRP binary...${NC}"
cd "$PROJECT_ROOT"
go build -o vrrp ./main.go

# Make scripts executable
chmod +x "$SCRIPT_DIR"/*.sh

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up test environment...${NC}"
    "$SCRIPT_DIR/teardown_namespace.sh"
    
    # Kill any remaining VRRP processes
    pkill -f "vrrp run" 2>/dev/null || true
    
    # Remove test binary
    rm -f "$PROJECT_ROOT/vrrp"
}

# Set trap to cleanup on exit
trap cleanup EXIT INT TERM

# Setup test namespaces
echo -e "${YELLOW}Setting up test namespaces...${NC}"
"$SCRIPT_DIR/setup_namespace.sh" ns1 veth1 10.0.0.1/24
"$SCRIPT_DIR/setup_namespace.sh" ns2 veth2 10.0.0.2/24

# Wait for interfaces to be ready
sleep 2

# Verify connectivity
echo -e "${YELLOW}Verifying namespace connectivity...${NC}"
if ip netns exec ns1 ping -c 1 -W 1 10.0.0.2 > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Namespace connectivity verified${NC}"
else
    echo -e "${RED}✗ Failed to establish namespace connectivity${NC}"
    exit 1
fi

# Run Go integration tests
echo -e "${YELLOW}Running Go integration tests...${NC}"
cd "$TEST_DIR"

# Set environment variables for tests
export VRRP_BIN="$PROJECT_ROOT/vrrp"
export TEST_NAMESPACES="ns1,ns2"
export TEST_BRIDGE="vrrp-br0"
export TEST_VIP="10.0.0.100"

# Run tests with timeout
if timeout 60 go test -v -tags integration ./... ; then
    echo -e "${GREEN}✓ All integration tests passed${NC}"
    exit 0
else
    echo -e "${RED}✗ Integration tests failed${NC}"
    exit 1
fi