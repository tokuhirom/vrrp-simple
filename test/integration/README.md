# VRRP Integration Tests

This directory contains integration tests for the VRRP implementation using Linux network namespaces.

## Prerequisites

- Linux operating system
- Root/sudo privileges
- Go 1.21 or later
- iproute2 package installed

## Running Tests

### Using Make (Recommended)

```bash
# Run all tests (unit + integration)
sudo make test-all

# Run only integration tests
sudo make integration-test
```

### Direct Execution

```bash
# Make scripts executable
chmod +x test/integration/scripts/*.sh

# Run integration tests
sudo ./test/integration/scripts/run_integration_tests.sh
```

### Individual Test Execution

```bash
# Build the binary first
go build -o vrrp ../../main.go

# Run specific test
sudo go test -v -tags integration -run TestMasterElection ./...
```

## Test Scenarios

1. **TestMasterElection**: Verifies that the instance with higher priority becomes master
2. **TestFailover**: Tests automatic failover when master fails
3. **TestPreemption**: Verifies that higher priority instance preempts lower priority master
4. **TestMultipleVRIDs**: Tests multiple virtual routers on the same interface

## Network Architecture

The tests create isolated network namespaces connected via a bridge:

```
Host System
├── ns1 (namespace 1)
│   ├── veth1: 10.0.0.1/24
│   └── VRRP instance
├── ns2 (namespace 2)
│   ├── veth2: 10.0.0.2/24
│   └── VRRP instance
└── vrrp-br0 (bridge)
    └── Virtual IP: 10.0.0.100
```

## Troubleshooting

### Permission Denied
Tests must be run as root:
```bash
sudo make integration-test
```

### Namespace Already Exists
Clean up existing namespaces:
```bash
sudo ./test/integration/scripts/teardown_namespace.sh
```

### View Network Namespaces
```bash
sudo ip netns list
```

### Debug Failed Tests
Check VRRP output in namespaces:
```bash
sudo ip netns exec ns1 ip addr show
sudo ip netns exec ns2 ip addr show
```

## Docker Testing

Alternatively, use Docker Compose for testing:

```bash
# Build and run tests
docker-compose -f docker-compose.test.yml up --build

# Clean up
docker-compose -f docker-compose.test.yml down
```

## CI/CD Integration

Tests automatically run on GitHub Actions for:
- Every push to main/develop branches
- All pull requests

See `.github/workflows/integration-test.yml` for CI configuration.