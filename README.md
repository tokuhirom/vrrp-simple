# vrrp-simple

A lightweight VRRP (Virtual Router Redundancy Protocol) implementation in Go that works both as a library and CLI tool.

## Features

- VRRP v2 protocol support
- Simple CLI interface using kingpin (no configuration files needed)
- Can be used as a Go library
- Master/Backup state machine
- Multicast communication (224.0.0.18)
- Priority-based master election
- Advertisement intervals
- Virtual IP management

## Installation

```bash
go get github.com/tokuhirom/vrrp-simple
```

## CLI Usage

### Running VRRP Instance

Run a VRRP instance with basic configuration:

```bash
# Run as master (priority 255)
sudo vrrp run --interface eth0 --vrid 10 --priority 255 --vips 192.168.1.100

# Run as backup (priority 100)
sudo vrrp run --interface eth0 --vrid 10 --priority 100 --vips 192.168.1.100

# Multiple virtual IPs
sudo vrrp run --interface eth0 --vrid 10 --priority 100 --vips 192.168.1.100,192.168.1.101

# Custom advertisement interval (default is 1 second)
sudo vrrp run --interface eth0 --vrid 10 --priority 100 --vips 192.168.1.100 --advert-int 3
```

### Command Line Options

```
vrrp run:
  -i, --interface    Network interface to use (required)
  -r, --vrid         Virtual Router ID 1-255 (required)
  -p, --priority     Router priority 1-255, 255=master (default: 100)
  -v, --vips         Virtual IP addresses, comma-separated (required)
  --advert-int       Advertisement interval in seconds (default: 1)
  --preempt          Enable preemption (default: true)
```

### Other Commands

```bash
# Show version
vrrp version

# Show status (placeholder for future implementation)
vrrp status --interface eth0 --vrid 10
```

## Library Usage

```go
package main

import (
    "log"
    "github.com/tokuhirom/vrrp-simple/pkg/vrrp"
)

func main() {
    config := &vrrp.Config{
        VRID:        10,
        Priority:    100,
        Interface:   "eth0",
        VirtualIPs:  []string{"192.168.1.100", "192.168.1.101"},
        AdvInterval: 1,
        Preempt:     true,
        Version:     vrrp.VRRPv2,
    }
    
    router, err := vrrp.NewVirtualRouter(config)
    if err != nil {
        log.Fatal(err)
    }
    
    if err := router.Start(); err != nil {
        log.Fatal(err)
    }
    
    // Check current state
    state := router.GetState()
    log.Printf("Current state: %s", state)
    
    // Stop when done
    defer router.Stop()
}
```

## Requirements

- Go 1.24.4 or later
- Root/Administrator privileges (for raw socket access)
- Linux operating system

## Protocol Details

This implementation follows RFC 3768 (VRRPv2) specifications:
- Uses IP protocol 112
- Multicast address: 224.0.0.18
- Default advertisement interval: 1 second
- Master down interval: 3 * Advertisement_Interval + Skew_time

## State Machine

The VRRP instance can be in one of three states:
- **INIT**: Initial state
- **BACKUP**: Backup router, monitoring for advertisements
- **MASTER**: Active router, sending advertisements and handling virtual IPs

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o vrrp cmd/vrrp/main.go
```

## Limitations

- Currently supports VRRPv2 only
- IPv4 support only
- Virtual IP management (adding/removing IPs from interface) is not fully implemented
- Status command requires IPC mechanism (planned for future)

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
