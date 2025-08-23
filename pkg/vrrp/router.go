package vrrp

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
)

type VirtualRouter struct {
	mu       sync.RWMutex
	vrid     uint8
	priority uint8
	ips      []net.IP
	iface    string
	
	network      *Network
	stateMachine *StateMachine
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	running bool
}

type Config struct {
	VRID        uint8
	Priority    uint8
	Interface   string
	VirtualIPs  []string
	AdvInterval int
	Preempt     bool
	Version     uint8
}

func NewVirtualRouter(cfg *Config) (*VirtualRouter, error) {
	if cfg.VRID == 0 || cfg.VRID > 255 {
		return nil, fmt.Errorf("invalid VRID: must be between 1 and 255")
	}
	
	if cfg.Priority > 255 {
		return nil, fmt.Errorf("invalid priority: must be between 0 and 255")
	}
	
	if cfg.Interface == "" {
		return nil, fmt.Errorf("interface name is required")
	}
	
	if len(cfg.VirtualIPs) == 0 {
		return nil, fmt.Errorf("at least one virtual IP is required")
	}
	
	ips := make([]net.IP, 0, len(cfg.VirtualIPs))
	for _, ipStr := range cfg.VirtualIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", ipStr)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("only IPv4 addresses are supported: %s", ipStr)
		}
		ips = append(ips, ip.To4())
	}
	
	return &VirtualRouter{
		vrid:     cfg.VRID,
		priority: cfg.Priority,
		ips:      ips,
		iface:    cfg.Interface,
	}, nil
}

func (vr *VirtualRouter) Start() error {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	
	if vr.running {
		return fmt.Errorf("virtual router is already running")
	}
	
	network, err := NewNetwork(vr.iface)
	if err != nil {
		return fmt.Errorf("failed to initialize network: %w", err)
	}
	vr.network = network
	
	vr.stateMachine = NewStateMachine(vr.vrid, vr.priority, vr.ips, vr.network.GetInterface())
	vr.stateMachine.SetStateChangeCallback(vr.onStateChange)
	
	vr.ctx, vr.cancel = context.WithCancel(context.Background())
	
	vr.wg.Add(2)
	go vr.sendLoop()
	go vr.recvLoop()
	
	if err := vr.stateMachine.Start(vr.ctx); err != nil {
		vr.cancel()
		vr.network.Close()
		return fmt.Errorf("failed to start state machine: %w", err)
	}
	
	vr.running = true
	log.Printf("Virtual Router started - VRID: %d, Priority: %d, Interface: %s", 
		vr.vrid, vr.priority, vr.iface)
	
	return nil
}

func (vr *VirtualRouter) Stop() error {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	
	if !vr.running {
		return fmt.Errorf("virtual router is not running")
	}
	
	vr.stateMachine.Stop()
	vr.cancel()
	
	vr.wg.Wait()
	
	if err := vr.network.Close(); err != nil {
		log.Printf("Failed to close network: %v", err)
	}
	
	vr.running = false
	log.Printf("Virtual Router stopped - VRID: %d", vr.vrid)
	
	return nil
}

func (vr *VirtualRouter) sendLoop() {
	defer vr.wg.Done()
	
	for {
		select {
		case <-vr.ctx.Done():
			return
			
		case pkt := <-vr.stateMachine.GetSendChannel():
			if err := vr.network.SendPacket(pkt); err != nil {
				log.Printf("Failed to send packet: %v", err)
			}
		}
	}
}

func (vr *VirtualRouter) recvLoop() {
	defer vr.wg.Done()
	
	err := vr.network.ReceivePackets(vr.ctx, func(pkt *Packet) {
		vr.stateMachine.ProcessPacket(pkt)
	})
	
	if err != nil && err != context.Canceled {
		log.Printf("Receive loop error: %v", err)
	}
}

func (vr *VirtualRouter) onStateChange(old, new State) {
	log.Printf("VRID %d: State changed from %s to %s", vr.vrid, old, new)
	
	if new == Master {
		log.Printf("VRID %d: Now MASTER for IPs: %v", vr.vrid, vr.ips)
	}
}

func (vr *VirtualRouter) GetState() State {
	if vr.stateMachine != nil {
		return vr.stateMachine.GetState()
	}
	return Init
}

func (vr *VirtualRouter) GetVRID() uint8 {
	return vr.vrid
}

func (vr *VirtualRouter) GetPriority() uint8 {
	return vr.priority
}

func (vr *VirtualRouter) GetVirtualIPs() []net.IP {
	return vr.ips
}

func (vr *VirtualRouter) IsRunning() bool {
	vr.mu.RLock()
	defer vr.mu.RUnlock()
	return vr.running
}