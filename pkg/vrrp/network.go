package vrrp

import (
	"context"
	"fmt"
	"log"
	"net"
	"syscall"

	"golang.org/x/net/ipv4"
)

const (
	VRRPMulticastIPv4 = "224.0.0.18"
	VRRPProtocol      = 112
)

type Network struct {
	iface    *net.Interface
	conn     *ipv4.RawConn
	sourceIP net.IP
}

func NewNetwork(ifaceName string) (*Network, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface %s: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get interface addresses: %w", err)
	}

	var sourceIP net.IP
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				sourceIP = ipv4
				break
			}
		}
	}

	if sourceIP == nil {
		return nil, fmt.Errorf("no IPv4 address found on interface %s", ifaceName)
	}

	conn, err := net.ListenPacket("ip4:112", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen for VRRP packets: %w", err)
	}

	rawConn, err := ipv4.NewRawConn(conn)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to create raw connection: %w", err)
	}

	if p, ok := conn.(*net.IPConn); ok {
		if err := p.SetReadBuffer(256 * 1024); err != nil {
			log.Printf("Failed to set read buffer: %v", err)
		}
		if err := p.SetWriteBuffer(256 * 1024); err != nil {
			log.Printf("Failed to set write buffer: %v", err)
		}
	}

	if err := joinMulticast(conn, iface); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to join multicast group: %w", err)
	}

	return &Network{
		iface:    iface,
		conn:     rawConn,
		sourceIP: sourceIP,
	}, nil
}

func joinMulticast(conn net.PacketConn, iface *net.Interface) error {
	group := net.ParseIP(VRRPMulticastIPv4)
	if group == nil {
		return fmt.Errorf("invalid multicast IP")
	}

	p := ipv4.NewPacketConn(conn)
	if err := p.JoinGroup(iface, &net.UDPAddr{IP: group}); err != nil {
		return err
	}

	if err := p.SetMulticastInterface(iface); err != nil {
		return err
	}

	if err := p.SetMulticastTTL(255); err != nil {
		return err
	}

	return nil
}

func (n *Network) Close() error {
	if n.conn != nil {
		return n.conn.Close()
	}
	return nil
}

func (n *Network) SendPacket(pkt *Packet) error {
	data, err := pkt.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal packet: %w", err)
	}

	dst := net.ParseIP(VRRPMulticastIPv4)
	if dst == nil {
		return fmt.Errorf("invalid destination IP")
	}

	header := &ipv4.Header{
		Version:  ipv4.Version,
		Len:      ipv4.HeaderLen,
		TOS:      0xc0,
		TotalLen: ipv4.HeaderLen + len(data),
		TTL:      255,
		Protocol: VRRPProtocol,
		Dst:      dst,
		Src:      n.sourceIP,
	}

	if err := n.conn.WriteTo(header, data, nil); err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}

	return nil
}

func (n *Network) ReceivePackets(ctx context.Context, handler func(*Packet)) error {
	buf := make([]byte, 1500)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		header, payload, _, err := n.conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if err == syscall.EINTR {
				continue
			}
			return fmt.Errorf("failed to read packet: %w", err)
		}

		if header.Protocol != VRRPProtocol {
			continue
		}

		pkt := &Packet{}
		if err := pkt.Unmarshal(payload); err != nil {
			log.Printf("Failed to unmarshal VRRP packet: %v", err)
			continue
		}

		handler(pkt)
	}
}

func (n *Network) GetInterface() *net.Interface {
	return n.iface
}

func (n *Network) GetSourceIP() net.IP {
	return n.sourceIP
}
