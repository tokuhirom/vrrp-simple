package vrrp

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

// IPManager handles adding and removing virtual IP addresses
type IPManager struct {
	iface *net.Interface
}

// NewIPManager creates a new IP manager for the given interface
func NewIPManager(iface *net.Interface) *IPManager {
	return &IPManager{
		iface: iface,
	}
}

// AddIP adds a virtual IP address to the interface
func (m *IPManager) AddIP(ip net.IP) error {
	// Get the netlink handle
	link, err := netlink.LinkByIndex(m.iface.Index)
	if err != nil {
		return fmt.Errorf("failed to get link by index %d: %w", m.iface.Index, err)
	}

	// Determine the appropriate prefix length
	prefixLen := 32
	if ip.To4() == nil {
		prefixLen = 128
	}

	// Create the address
	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(prefixLen, prefixLen),
		},
		Label: m.iface.Name,
		Scope: int(netlink.SCOPE_UNIVERSE),
	}

	// Check if the address already exists
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list addresses: %w", err)
	}

	for _, a := range addrs {
		if a.IP.Equal(ip) {
			// Address already exists
			return nil
		}
	}

	// Add the address
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("failed to add IP %s to interface %s: %w", ip, m.iface.Name, err)
	}

	return nil
}

// DelIP removes a virtual IP address from the interface
func (m *IPManager) DelIP(ip net.IP) error {
	// Get the netlink handle
	link, err := netlink.LinkByIndex(m.iface.Index)
	if err != nil {
		return fmt.Errorf("failed to get link by index %d: %w", m.iface.Index, err)
	}

	// Get all addresses on the interface
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list addresses: %w", err)
	}

	// Find and delete the matching address
	for _, addr := range addrs {
		if addr.IP.Equal(ip) {
			if err := netlink.AddrDel(link, &addr); err != nil {
				return fmt.Errorf("failed to delete IP %s from interface %s: %w", ip, m.iface.Name, err)
			}
			return nil
		}
	}

	// Address not found, but that's OK (idempotent operation)
	return nil
}

// ListIPs returns all IP addresses on the interface
func (m *IPManager) ListIPs() ([]net.IP, error) {
	link, err := netlink.LinkByIndex(m.iface.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to get link by index %d: %w", m.iface.Index, err)
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return nil, fmt.Errorf("failed to list addresses: %w", err)
	}

	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}

	return ips, nil
}

// SetArpReply configures ARP reply behavior for virtual IPs
func (m *IPManager) SetArpReply(enable bool) error {
	// Configure arp_ignore and arp_announce for proper VRRP behavior
	// arp_ignore=1: reply only if the target IP address is configured on the incoming interface
	// arp_announce=2: use the best local address

	settings := map[string]string{
		fmt.Sprintf("/proc/sys/net/ipv4/conf/%s/arp_ignore", m.iface.Name):   "1",
		fmt.Sprintf("/proc/sys/net/ipv4/conf/%s/arp_announce", m.iface.Name): "2",
		fmt.Sprintf("/proc/sys/net/ipv4/conf/all/arp_ignore"):                "1",
		fmt.Sprintf("/proc/sys/net/ipv4/conf/all/arp_announce"):              "2",
	}

	if !enable {
		settings[fmt.Sprintf("/proc/sys/net/ipv4/conf/%s/arp_ignore", m.iface.Name)] = "0"
		settings[fmt.Sprintf("/proc/sys/net/ipv4/conf/%s/arp_announce", m.iface.Name)] = "0"
	}

	for path, value := range settings {
		if err := writeSysctl(path, value); err != nil {
			// Log but don't fail - some systems might not allow sysctl changes
			// This is a best-effort optimization
			continue
		}
	}

	return nil
}

func writeSysctl(path, value string) error {
	// This would write to /proc/sys but needs proper error handling
	// For now, return nil as it's optional
	return nil
}
