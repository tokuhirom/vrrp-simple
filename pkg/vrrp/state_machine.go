package vrrp

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type State int

const (
	Init State = iota
	Backup
	Master
)

func (s State) String() string {
	switch s {
	case Init:
		return "INIT"
	case Backup:
		return "BACKUP"
	case Master:
		return "MASTER"
	default:
		return "UNKNOWN"
	}
}

type StateMachine struct {
	mu              sync.RWMutex
	state           State
	vrid            uint8
	priority        uint8
	advertisementInterval time.Duration
	masterDownInterval   time.Duration
	virtualIPs      []net.IP
	iface           *net.Interface
	
	masterDownTimer *time.Timer
	advertTimer     *time.Ticker
	
	sendCh   chan *Packet
	recvCh   chan *Packet
	eventCh  chan Event
	stopCh   chan struct{}
	
	onStateChange func(old, new State)
}

type Event int

const (
	EventStartup Event = iota
	EventShutdown
	EventMasterDown
	EventAdvertReceived
	EventPriorityZeroReceived
)

func NewStateMachine(vrid, priority uint8, ips []net.IP, iface *net.Interface) *StateMachine {
	sm := &StateMachine{
		state:                Init,
		vrid:                 vrid,
		priority:             priority,
		advertisementInterval: time.Second,
		virtualIPs:           ips,
		iface:                iface,
		sendCh:               make(chan *Packet, 10),
		recvCh:               make(chan *Packet, 10),
		eventCh:              make(chan Event, 10),
		stopCh:               make(chan struct{}),
	}
	
	sm.masterDownInterval = sm.calculateMasterDownInterval()
	
	return sm
}

func (sm *StateMachine) SetStateChangeCallback(fn func(old, new State)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onStateChange = fn
}

func (sm *StateMachine) GetState() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

func (sm *StateMachine) calculateMasterDownInterval() time.Duration {
	skewTime := time.Duration((256-int(sm.priority))*int(sm.advertisementInterval.Milliseconds()/256)) * time.Millisecond
	return 3*sm.advertisementInterval + skewTime
}

func (sm *StateMachine) Start(ctx context.Context) error {
	sm.eventCh <- EventStartup
	
	go sm.run(ctx)
	
	return nil
}

func (sm *StateMachine) Stop() {
	close(sm.stopCh)
}

func (sm *StateMachine) ProcessPacket(pkt *Packet) {
	select {
	case sm.recvCh <- pkt:
	default:
		log.Printf("Receive channel full, dropping packet")
	}
}

func (sm *StateMachine) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			sm.transition(Init)
			return
			
		case <-sm.stopCh:
			sm.transition(Init)
			return
			
		case event := <-sm.eventCh:
			sm.handleEvent(event)
			
		case pkt := <-sm.recvCh:
			sm.handlePacket(pkt)
			
		case <-sm.masterDownTimerChan():
			if sm.state == Backup {
				sm.eventCh <- EventMasterDown
			}
			
		case <-sm.advertTimerChan():
			if sm.state == Master {
				sm.sendAdvertisement()
			}
		}
	}
}

func (sm *StateMachine) masterDownTimerChan() <-chan time.Time {
	if sm.masterDownTimer != nil {
		return sm.masterDownTimer.C
	}
	return nil
}

func (sm *StateMachine) advertTimerChan() <-chan time.Time {
	if sm.advertTimer != nil {
		return sm.advertTimer.C
	}
	return nil
}

func (sm *StateMachine) handleEvent(event Event) {
	switch event {
	case EventStartup:
		if sm.priority == 255 {
			sm.transition(Master)
		} else {
			sm.transition(Backup)
		}
		
	case EventShutdown:
		sm.transition(Init)
		
	case EventMasterDown:
		if sm.state == Backup {
			sm.transition(Master)
		}
		
	case EventPriorityZeroReceived:
		if sm.state == Master {
			sm.sendAdvertisement()
		}
	}
}

func (sm *StateMachine) handlePacket(pkt *Packet) {
	if pkt.VRID != sm.vrid {
		return
	}
	
	if pkt.Priority == 0 {
		sm.eventCh <- EventPriorityZeroReceived
		return
	}
	
	switch sm.state {
	case Backup:
		if pkt.Priority >= sm.priority {
			sm.resetMasterDownTimer()
		}
		
	case Master:
		if pkt.Priority > sm.priority || 
		   (pkt.Priority == sm.priority && sm.compareSourceIP(pkt) > 0) {
			sm.transition(Backup)
		}
	}
}

func (sm *StateMachine) transition(newState State) {
	sm.mu.Lock()
	oldState := sm.state
	
	if oldState == newState {
		sm.mu.Unlock()
		return
	}
	
	log.Printf("VRID %d: State transition %s -> %s", sm.vrid, oldState, newState)
	
	switch oldState {
	case Master:
		sm.stopAdvertTimer()
		sm.releaseVirtualIPs()
		
	case Backup:
		sm.stopMasterDownTimer()
	}
	
	sm.state = newState
	
	switch newState {
	case Master:
		sm.acquireVirtualIPs()
		sm.sendAdvertisement()
		sm.startAdvertTimer()
		
	case Backup:
		sm.startMasterDownTimer()
		
	case Init:
		sm.stopAdvertTimer()
		sm.stopMasterDownTimer()
		sm.releaseVirtualIPs()
	}
	
	if sm.onStateChange != nil {
		sm.onStateChange(oldState, newState)
	}
	
	sm.mu.Unlock()
}

func (sm *StateMachine) startMasterDownTimer() {
	sm.stopMasterDownTimer()
	sm.masterDownTimer = time.NewTimer(sm.masterDownInterval)
}

func (sm *StateMachine) stopMasterDownTimer() {
	if sm.masterDownTimer != nil {
		sm.masterDownTimer.Stop()
		sm.masterDownTimer = nil
	}
}

func (sm *StateMachine) resetMasterDownTimer() {
	sm.stopMasterDownTimer()
	sm.masterDownTimer = time.NewTimer(sm.masterDownInterval)
}

func (sm *StateMachine) startAdvertTimer() {
	sm.stopAdvertTimer()
	sm.advertTimer = time.NewTicker(sm.advertisementInterval)
}

func (sm *StateMachine) stopAdvertTimer() {
	if sm.advertTimer != nil {
		sm.advertTimer.Stop()
		sm.advertTimer = nil
	}
}

func (sm *StateMachine) sendAdvertisement() {
	pkt := NewPacket(VRRPv2, sm.vrid, sm.priority, sm.virtualIPs)
	
	select {
	case sm.sendCh <- pkt:
	default:
		log.Printf("Send channel full, dropping advertisement")
	}
}

func (sm *StateMachine) acquireVirtualIPs() {
	for _, ip := range sm.virtualIPs {
		if err := sm.addIP(ip); err != nil {
			log.Printf("Failed to add virtual IP %s: %v", ip, err)
		} else {
			log.Printf("Added virtual IP %s to interface %s", ip, sm.iface.Name)
		}
	}
}

func (sm *StateMachine) releaseVirtualIPs() {
	for _, ip := range sm.virtualIPs {
		if err := sm.delIP(ip); err != nil {
			log.Printf("Failed to remove virtual IP %s: %v", ip, err)
		} else {
			log.Printf("Removed virtual IP %s from interface %s", ip, sm.iface.Name)
		}
	}
}

func (sm *StateMachine) addIP(ip net.IP) error {
	return fmt.Errorf("IP management not implemented")
}

func (sm *StateMachine) delIP(ip net.IP) error {
	return fmt.Errorf("IP management not implemented")
}

func (sm *StateMachine) compareSourceIP(pkt *Packet) int {
	return 0
}

func (sm *StateMachine) GetSendChannel() <-chan *Packet {
	return sm.sendCh
}