#!/bin/bash
set -e

# Setup network namespaces for VRRP testing
# Usage: ./setup_namespace.sh <namespace_name> <veth_name> <ip_address>

NS_NAME=${1:-ns1}
VETH_NAME=${2:-veth1}
IP_ADDR=${3:-10.0.0.1/24}
BRIDGE_NAME=${4:-vrrp-br0}
VETH_PEER="${VETH_NAME}-peer"

echo "Setting up namespace: $NS_NAME with interface $VETH_NAME ($IP_ADDR)"

# Create namespace if it doesn't exist
if ! ip netns list | grep -q "^$NS_NAME"; then
    ip netns add "$NS_NAME"
    echo "Created namespace: $NS_NAME"
fi

# Create veth pair if it doesn't exist
if ! ip link show "$VETH_NAME" 2>/dev/null; then
    ip link add "$VETH_NAME" type veth peer name "$VETH_PEER"
    echo "Created veth pair: $VETH_NAME <-> $VETH_PEER"
fi

# Move veth into namespace
ip link set "$VETH_NAME" netns "$NS_NAME"

# Setup bridge if it doesn't exist
if ! ip link show "$BRIDGE_NAME" 2>/dev/null; then
    ip link add "$BRIDGE_NAME" type bridge
    ip link set "$BRIDGE_NAME" up
    ip addr add 10.0.0.254/24 dev "$BRIDGE_NAME" 2>/dev/null || true
    echo "Created bridge: $BRIDGE_NAME"
fi

# Connect peer to bridge
ip link set "$VETH_PEER" master "$BRIDGE_NAME"
ip link set "$VETH_PEER" up

# Configure interface in namespace
ip netns exec "$NS_NAME" ip link set lo up
ip netns exec "$NS_NAME" ip link set "$VETH_NAME" up
ip netns exec "$NS_NAME" ip addr add "$IP_ADDR" dev "$VETH_NAME"

# Enable IP forwarding and multicast
ip netns exec "$NS_NAME" sysctl -w net.ipv4.ip_forward=1 >/dev/null
ip netns exec "$NS_NAME" sysctl -w net.ipv4.conf.all.rp_filter=0 >/dev/null
ip netns exec "$NS_NAME" sysctl -w net.ipv4.conf."$VETH_NAME".rp_filter=0 >/dev/null

# Add multicast route for VRRP
ip netns exec "$NS_NAME" ip route add 224.0.0.0/4 dev "$VETH_NAME" 2>/dev/null || true

echo "Namespace $NS_NAME setup complete"
echo "  Interface: $VETH_NAME"
echo "  IP: $IP_ADDR"
echo "  Bridge: $BRIDGE_NAME"