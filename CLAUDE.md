# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a VRRP (Virtual Router Redundancy Protocol) implementation in Go that works both as a library and CLI tool. The project implements VRRPv2 (RFC 3768) with real IP management using netlink.

Key design decision: No configuration files - all configuration is done via command-line flags using kingpin.

## Build and Test Commands

```bash
# Build
make build                    # Creates ./vrrp binary
go build -o vrrp ./main.go   # Alternative

# Unit tests
make test                     # Run with race detection
go test -v ./pkg/vrrp -run TestSpecificName  # Run single test

# Integration tests (requires root for network namespaces)
sudo make integration-test    # Uses network namespaces
sudo go test -v ./pkg/vrrp -run TestIPManager  # Test IP management

# Coverage
make test-coverage           # Generates coverage.html

# Linting (uses golangci-lint v2 with TOML config issue - use --no-config)
golangci-lint run --no-config ./...
make lint

# Format code
goimports -w -local github.com/tokuhirom/vrrp-simple .
```

## Architecture

### Core Components

**pkg/vrrp/** - Library implementation
- `packet.go` - VRRP packet marshaling/unmarshaling (VRRPv2 protocol)
- `state_machine.go` - VRRP state transitions (Init→Backup→Master)
  - Uses channels for event-driven architecture
  - Master election with source IP tie-breaking
- `network.go` - Raw socket multicast (224.0.0.18, IP protocol 112)
- `router.go` - VirtualRouter orchestrates state machine + network
- `ip_manager.go` - Virtual IP management via netlink (requires root)

**main.go** - CLI using kingpin
- `vrrp run` - Start VRRP instance
- `vrrp status` - Not implemented (placeholder)
- `vrrp version` - Show version

### State Machine Flow

1. **Init State**: Starting point
2. **Backup State**: Monitors for advertisements, has master down timer
3. **Master State**: Sends advertisements, manages virtual IPs

Priority 255 = always master. Same priority uses source IP comparison for tie-breaking.

### Network Layer

- Uses raw IP sockets (requires root/CAP_NET_RAW)
- Multicast group 224.0.0.18
- IP protocol 112 (VRRP)
- Implements packet checksum calculation

### IP Management

Uses github.com/vishvananda/netlink to add/remove IPs from interfaces. Operations are idempotent.

## Testing Strategy

**Unit Tests**: Mock interfaces, test packet encoding, state transitions
**Integration Tests**: Use Linux network namespaces to create isolated test networks

Test infrastructure in `test/integration/`:
- Creates namespaces (ns1, ns2) connected via bridge
- Runs actual VRRP instances
- Tests master election, failover, preemption

## Git Hooks (Lefthook)

Pre-commit: goimports, golangci-lint, go mod tidy
Pre-push: tests, build verification, full lint
Commit-msg: Enforces conventional commit format

## Known Issues

1. `golangci-lint` config format issue - use `--no-config` flag
2. `vrrp status` command not implemented 
3. `writeSysctl()` in ip_manager.go returns nil (ARP optimization not critical)
4. README incorrectly states IP management not implemented (it is)

## Protocol Limitations

- VRRPv2 only (no v3)
- IPv4 only (no IPv6)
- No authentication support
- No health check triggers

## Required Permissions

Must run as root or with CAP_NET_RAW + CAP_NET_ADMIN for:
- Raw socket access (VRRP packets)
- IP address management (netlink)
- Network namespace operations (tests)