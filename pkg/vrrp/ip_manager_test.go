package vrrp

import (
	"net"
	"os"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestIPManager(t *testing.T) {
	// Skip if not running as root
	if os.Geteuid() != 0 {
		t.Skip("Test requires root privileges")
	}

	// Create a dummy interface for testing
	dummy := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name: "vrrp-test-dummy",
		},
	}

	// Add the dummy interface
	if err := netlink.LinkAdd(dummy); err != nil {
		t.Fatalf("Failed to create dummy interface: %v", err)
	}
	defer netlink.LinkDel(dummy)

	// Bring the interface up
	if err := netlink.LinkSetUp(dummy); err != nil {
		t.Fatalf("Failed to bring up dummy interface: %v", err)
	}

	// Get the interface
	iface, err := net.InterfaceByName("vrrp-test-dummy")
	if err != nil {
		t.Fatalf("Failed to get interface: %v", err)
	}

	// Create IP manager
	ipMgr := NewIPManager(iface)

	testIP := net.ParseIP("192.168.100.100")
	if testIP == nil {
		t.Fatal("Failed to parse test IP")
	}

	// Test adding IP
	t.Run("AddIP", func(t *testing.T) {
		if err := ipMgr.AddIP(testIP); err != nil {
			t.Errorf("Failed to add IP: %v", err)
		}

		// Verify IP was added
		ips, err := ipMgr.ListIPs()
		if err != nil {
			t.Errorf("Failed to list IPs: %v", err)
		}

		found := false
		for _, ip := range ips {
			if ip.Equal(testIP) {
				found = true
				break
			}
		}

		if !found {
			t.Error("IP was not added to interface")
		}
	})

	// Test adding duplicate IP (should be idempotent)
	t.Run("AddDuplicateIP", func(t *testing.T) {
		if err := ipMgr.AddIP(testIP); err != nil {
			t.Errorf("Failed to add duplicate IP: %v", err)
		}
	})

	// Test deleting IP
	t.Run("DelIP", func(t *testing.T) {
		if err := ipMgr.DelIP(testIP); err != nil {
			t.Errorf("Failed to delete IP: %v", err)
		}

		// Verify IP was removed
		ips, err := ipMgr.ListIPs()
		if err != nil {
			t.Errorf("Failed to list IPs: %v", err)
		}

		found := false
		for _, ip := range ips {
			if ip.Equal(testIP) {
				found = true
				break
			}
		}

		if found {
			t.Error("IP was not removed from interface")
		}
	})

	// Test deleting non-existent IP (should be idempotent)
	t.Run("DelNonExistentIP", func(t *testing.T) {
		if err := ipMgr.DelIP(testIP); err != nil {
			t.Errorf("Failed to delete non-existent IP: %v", err)
		}
	})

	// Test multiple IPs
	t.Run("MultipleIPs", func(t *testing.T) {
		ip1 := net.ParseIP("10.0.0.100")
		ip2 := net.ParseIP("10.0.0.101")
		ip3 := net.ParseIP("10.0.0.102")

		// Add multiple IPs
		for _, ip := range []net.IP{ip1, ip2, ip3} {
			if err := ipMgr.AddIP(ip); err != nil {
				t.Errorf("Failed to add IP %s: %v", ip, err)
			}
		}

		// List and verify
		ips, err := ipMgr.ListIPs()
		if err != nil {
			t.Errorf("Failed to list IPs: %v", err)
		}

		// Count our test IPs
		count := 0
		for _, ip := range ips {
			if ip.Equal(ip1) || ip.Equal(ip2) || ip.Equal(ip3) {
				count++
			}
		}

		if count != 3 {
			t.Errorf("Expected 3 test IPs, found %d", count)
		}

		// Clean up
		for _, ip := range []net.IP{ip1, ip2, ip3} {
			if err := ipMgr.DelIP(ip); err != nil {
				t.Errorf("Failed to delete IP %s: %v", ip, err)
			}
		}
	})
}

func TestIPManagerWithoutRoot(t *testing.T) {
	// Test that we handle non-root gracefully
	if os.Geteuid() == 0 {
		t.Skip("Test requires non-root privileges")
	}

	// Try to get a real interface
	ifaces, err := net.Interfaces()
	if err != nil || len(ifaces) == 0 {
		t.Skip("No interfaces available")
	}

	ipMgr := NewIPManager(&ifaces[0])
	testIP := net.ParseIP("192.168.200.200")

	// These should fail without root
	err = ipMgr.AddIP(testIP)
	if err == nil {
		t.Error("Expected error when adding IP without root")
	}

	err = ipMgr.DelIP(testIP)
	// DelIP might not error if IP doesn't exist
	_ = err
}
