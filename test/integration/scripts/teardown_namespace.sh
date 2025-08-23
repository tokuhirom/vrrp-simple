#!/bin/bash
set -e

# Teardown network namespaces used for VRRP testing
# Usage: ./teardown_namespace.sh [namespace_name]

NS_NAME=${1}
BRIDGE_NAME=${2:-vrrp-br0}

teardown_namespace() {
    local ns=$1
    
    if ip netns list | grep -q "^$ns"; then
        echo "Removing namespace: $ns"
        
        # Find and remove veth pairs connected to this namespace
        for veth in $(ip netns exec "$ns" ip link show | grep '@' | cut -d: -f2 | cut -d@ -f1); do
            ip netns exec "$ns" ip link delete "$veth" 2>/dev/null || true
        done
        
        # Delete the namespace
        ip netns delete "$ns"
        echo "Namespace $ns removed"
    fi
}

teardown_bridge() {
    if ip link show "$BRIDGE_NAME" 2>/dev/null; then
        echo "Removing bridge: $BRIDGE_NAME"
        
        # Remove all interfaces from bridge
        for iface in $(ip link show master "$BRIDGE_NAME" | grep -E '^[0-9]+:' | cut -d: -f2); do
            ip link set "$iface" nomaster 2>/dev/null || true
            ip link delete "$iface" 2>/dev/null || true
        done
        
        # Delete the bridge
        ip link delete "$BRIDGE_NAME"
        echo "Bridge $BRIDGE_NAME removed"
    fi
}

# If specific namespace provided, remove only that
if [ -n "$NS_NAME" ]; then
    teardown_namespace "$NS_NAME"
else
    # Remove all test namespaces
    echo "Cleaning up all test namespaces..."
    
    for ns in $(ip netns list | grep -E '^(ns[0-9]+|vrrp-test-)' | cut -d' ' -f1); do
        teardown_namespace "$ns"
    done
    
    # Remove bridge
    teardown_bridge
fi

# Clean up any orphaned veth interfaces
for veth in $(ip link show | grep -E 'veth[0-9]+-peer' | cut -d: -f2); do
    ip link delete "$veth" 2>/dev/null || true
done

echo "Cleanup complete"