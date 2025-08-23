package vrrp

import (
	"encoding/binary"
	"fmt"
	"net"
)

const (
	VRRPv2 = 2
	VRRPv3 = 3
)

const (
	TypeAdvertisement = 1
)

const (
	StateInit   = 1
	StateBackup = 2
	StateMaster = 3
)

type Packet struct {
	Version      uint8
	Type         uint8
	VRID         uint8
	Priority     uint8
	CountIPAddrs uint8
	AuthType     uint8
	AdvInterval  uint8
	Checksum     uint16
	IPAddresses  []net.IP
	AuthData     []byte
}

func NewPacket(version, vrid, priority uint8, ips []net.IP) *Packet {
	return &Packet{
		Version:      version,
		Type:         TypeAdvertisement,
		VRID:         vrid,
		Priority:     priority,
		CountIPAddrs: uint8(len(ips)),
		IPAddresses:  ips,
		AdvInterval:  1,
	}
}

func (p *Packet) Marshal() ([]byte, error) {
	if p.Version != VRRPv2 && p.Version != VRRPv3 {
		return nil, fmt.Errorf("unsupported VRRP version: %d", p.Version)
	}

	size := 8
	for _, ip := range p.IPAddresses {
		if ip.To4() != nil {
			size += 4
		} else {
			size += 16
		}
	}

	if p.Version == VRRPv2 {
		size += 8
	}

	buf := make([]byte, size)
	
	buf[0] = (p.Version << 4) | (p.Type & 0x0F)
	buf[1] = p.VRID
	buf[2] = p.Priority
	buf[3] = p.CountIPAddrs
	
	if p.Version == VRRPv2 {
		buf[4] = p.AuthType
		buf[5] = p.AdvInterval
	} else {
		buf[4] = (p.AdvInterval >> 4) & 0xF0
		buf[5] = p.AdvInterval & 0xFF
	}
	
	offset := 8
	for _, ip := range p.IPAddresses {
		if v4 := ip.To4(); v4 != nil {
			copy(buf[offset:], v4)
			offset += 4
		} else {
			copy(buf[offset:], ip.To16())
			offset += 16
		}
	}
	
	if p.Version == VRRPv2 && len(p.AuthData) == 8 {
		copy(buf[offset:], p.AuthData)
	}
	
	checksum := p.calculateChecksum(buf)
	binary.BigEndian.PutUint16(buf[6:8], checksum)
	
	return buf, nil
}

func (p *Packet) Unmarshal(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("packet too short: %d bytes", len(data))
	}
	
	p.Version = (data[0] >> 4) & 0x0F
	p.Type = data[0] & 0x0F
	p.VRID = data[1]
	p.Priority = data[2]
	p.CountIPAddrs = data[3]
	
	if p.Version == VRRPv2 {
		p.AuthType = data[4]
		p.AdvInterval = data[5]
	} else {
		p.AdvInterval = ((data[4] & 0x0F) << 8) | data[5]
	}
	
	p.Checksum = binary.BigEndian.Uint16(data[6:8])
	
	offset := 8
	p.IPAddresses = make([]net.IP, p.CountIPAddrs)
	
	for i := 0; i < int(p.CountIPAddrs); i++ {
		if p.Version == VRRPv2 || (p.Version == VRRPv3 && len(data[offset:]) >= 4) {
			if offset+4 > len(data) {
				return fmt.Errorf("insufficient data for IPv4 address")
			}
			p.IPAddresses[i] = net.IP(data[offset : offset+4])
			offset += 4
		} else {
			if offset+16 > len(data) {
				return fmt.Errorf("insufficient data for IPv6 address")
			}
			p.IPAddresses[i] = net.IP(data[offset : offset+16])
			offset += 16
		}
	}
	
	if p.Version == VRRPv2 && offset+8 <= len(data) {
		p.AuthData = data[offset : offset+8]
	}
	
	return nil
}

func (p *Packet) calculateChecksum(data []byte) uint16 {
	temp := make([]byte, len(data))
	copy(temp, data)
	temp[6] = 0
	temp[7] = 0
	
	var sum uint32
	for i := 0; i < len(temp)-1; i += 2 {
		sum += uint32(temp[i])<<8 + uint32(temp[i+1])
	}
	
	if len(temp)%2 != 0 {
		sum += uint32(temp[len(temp)-1]) << 8
	}
	
	for (sum >> 16) > 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	
	return uint16(^sum)
}