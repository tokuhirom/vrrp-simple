package vrrp

import (
	"net"
	"testing"
)

func TestNewPacket(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("192.168.1.100"),
		net.ParseIP("192.168.1.101"),
	}
	
	pkt := NewPacket(VRRPv2, 1, 100, ips)
	
	if pkt.Version != VRRPv2 {
		t.Errorf("Expected version %d, got %d", VRRPv2, pkt.Version)
	}
	
	if pkt.Type != TypeAdvertisement {
		t.Errorf("Expected type %d, got %d", TypeAdvertisement, pkt.Type)
	}
	
	if pkt.VRID != 1 {
		t.Errorf("Expected VRID 1, got %d", pkt.VRID)
	}
	
	if pkt.Priority != 100 {
		t.Errorf("Expected priority 100, got %d", pkt.Priority)
	}
	
	if pkt.CountIPAddrs != 2 {
		t.Errorf("Expected 2 IP addresses, got %d", pkt.CountIPAddrs)
	}
	
	if len(pkt.IPAddresses) != 2 {
		t.Errorf("Expected 2 IP addresses in slice, got %d", len(pkt.IPAddresses))
	}
}

func TestPacketMarshalUnmarshal(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("192.168.1.100").To4(),
		net.ParseIP("192.168.1.101").To4(),
	}
	
	original := NewPacket(VRRPv2, 10, 150, ips)
	original.AuthType = 0
	original.AdvInterval = 1
	
	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Failed to marshal packet: %v", err)
	}
	
	if len(data) < 20 {
		t.Errorf("Marshaled data too short: %d bytes", len(data))
	}
	
	decoded := &Packet{}
	err = decoded.Unmarshal(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal packet: %v", err)
	}
	
	if decoded.Version != original.Version {
		t.Errorf("Version mismatch: expected %d, got %d", original.Version, decoded.Version)
	}
	
	if decoded.Type != original.Type {
		t.Errorf("Type mismatch: expected %d, got %d", original.Type, decoded.Type)
	}
	
	if decoded.VRID != original.VRID {
		t.Errorf("VRID mismatch: expected %d, got %d", original.VRID, decoded.VRID)
	}
	
	if decoded.Priority != original.Priority {
		t.Errorf("Priority mismatch: expected %d, got %d", original.Priority, decoded.Priority)
	}
	
	if decoded.CountIPAddrs != original.CountIPAddrs {
		t.Errorf("CountIPAddrs mismatch: expected %d, got %d", original.CountIPAddrs, decoded.CountIPAddrs)
	}
	
	if len(decoded.IPAddresses) != len(original.IPAddresses) {
		t.Fatalf("IP address count mismatch: expected %d, got %d", 
			len(original.IPAddresses), len(decoded.IPAddresses))
	}
	
	for i, ip := range decoded.IPAddresses {
		if !ip.Equal(original.IPAddresses[i]) {
			t.Errorf("IP address %d mismatch: expected %s, got %s", 
				i, original.IPAddresses[i], ip)
		}
	}
}

func TestPacketMarshalInvalidVersion(t *testing.T) {
	pkt := &Packet{
		Version: 99,
		Type:    TypeAdvertisement,
		VRID:    1,
	}
	
	_, err := pkt.Marshal()
	if err == nil {
		t.Error("Expected error for invalid version, got nil")
	}
}

func TestPacketUnmarshalShortData(t *testing.T) {
	data := []byte{0x21, 0x01, 0x64}
	
	pkt := &Packet{}
	err := pkt.Unmarshal(data)
	if err == nil {
		t.Error("Expected error for short data, got nil")
	}
}

func TestPacketChecksum(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("10.0.0.1").To4(),
	}
	
	pkt := NewPacket(VRRPv2, 1, 200, ips)
	
	data, err := pkt.Marshal()
	if err != nil {
		t.Fatalf("Failed to marshal packet: %v", err)
	}
	
	checksum := (uint16(data[6]) << 8) | uint16(data[7])
	if checksum == 0 {
		t.Error("Checksum should not be zero")
	}
	
	decoded := &Packet{}
	err = decoded.Unmarshal(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal packet: %v", err)
	}
	
	if decoded.Checksum != checksum {
		t.Errorf("Checksum mismatch: expected %d, got %d", checksum, decoded.Checksum)
	}
}