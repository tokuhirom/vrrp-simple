package vrrp

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestStateMachineSourceIPComparison(t *testing.T) {
	// Create a test interface (we'll mock the actual interface operations)
	iface := &net.Interface{
		Index: 1,
		Name:  "test0",
	}

	// Create state machine with source IP
	vips := []net.IP{net.ParseIP("192.168.1.100")}
	sm := NewStateMachine(1, 100, vips, iface)
	sm.sourceIP = net.ParseIP("10.0.0.1")

	tests := []struct {
		name           string
		packetIP       net.IP
		expectedResult int
		description    string
	}{
		{
			name:           "Packet IP greater",
			packetIP:       net.ParseIP("10.0.0.2"),
			expectedResult: -1,
			description:    "Packet has higher IP, should win tie-break",
		},
		{
			name:           "Packet IP lesser",
			packetIP:       net.ParseIP("10.0.0.0"),
			expectedResult: 1,
			description:    "We have higher IP, should win tie-break",
		},
		{
			name:           "Equal IPs",
			packetIP:       net.ParseIP("10.0.0.1"),
			expectedResult: 0,
			description:    "Same IP, no winner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := &Packet{
				IPAddresses: []net.IP{tt.packetIP},
			}

			result := sm.compareSourceIP(pkt)
			if result != tt.expectedResult {
				t.Errorf("%s: expected %d, got %d", tt.description, tt.expectedResult, result)
			}
		})
	}
}

func TestStateMachineWithNoSourceIP(t *testing.T) {
	iface := &net.Interface{
		Index: 1,
		Name:  "test0",
	}

	vips := []net.IP{net.ParseIP("192.168.1.100")}
	sm := NewStateMachine(1, 100, vips, iface)
	sm.sourceIP = nil // Explicitly set to nil

	pkt := &Packet{
		IPAddresses: []net.IP{net.ParseIP("10.0.0.1")},
	}

	result := sm.compareSourceIP(pkt)
	if result != -1 {
		t.Errorf("With no source IP, should always lose tie-break, got %d", result)
	}
}

func TestStateMachineTransitions(t *testing.T) {
	iface := &net.Interface{
		Index: 1,
		Name:  "test0",
	}

	vips := []net.IP{net.ParseIP("192.168.1.100")}
	sm := NewStateMachine(1, 100, vips, iface)

	// Test initial state
	if sm.GetState() != Init {
		t.Errorf("Initial state should be Init, got %v", sm.GetState())
	}

	// Test state change callback
	var oldState, newState State
	sm.SetStateChangeCallback(func(old, new State) {
		oldState = old
		newState = new
	})

	// Transition to Backup
	sm.transition(Backup)
	if sm.GetState() != Backup {
		t.Errorf("State should be Backup, got %v", sm.GetState())
	}
	if oldState != Init || newState != Backup {
		t.Errorf("Callback not called correctly: old=%v, new=%v", oldState, newState)
	}

	// Transition to Master
	sm.transition(Master)
	if sm.GetState() != Master {
		t.Errorf("State should be Master, got %v", sm.GetState())
	}
	if oldState != Backup || newState != Master {
		t.Errorf("Callback not called correctly: old=%v, new=%v", oldState, newState)
	}
}

func TestMasterDownInterval(t *testing.T) {
	iface := &net.Interface{
		Index: 1,
		Name:  "test0",
	}

	vips := []net.IP{net.ParseIP("192.168.1.100")}

	// Test with different priorities
	priorities := []uint8{1, 100, 200, 254, 255}

	for _, priority := range priorities {
		sm := NewStateMachine(1, priority, vips, iface)
		interval := sm.calculateMasterDownInterval()

		// Master down interval should be 3 * advert_interval + skew_time
		// Skew time = (256 - priority) * advert_interval / 256
		expectedSkew := time.Duration((256-int(priority))*int(sm.advertisementInterval.Milliseconds()/256)) * time.Millisecond
		expected := 3*sm.advertisementInterval + expectedSkew

		if interval != expected {
			t.Errorf("Priority %d: expected interval %v, got %v", priority, expected, interval)
		}
	}
}

func TestPacketHandling(t *testing.T) {
	iface := &net.Interface{
		Index: 1,
		Name:  "test0",
	}

	vips := []net.IP{net.ParseIP("192.168.1.100")}
	sm := NewStateMachine(10, 100, vips, iface)
	sm.sourceIP = net.ParseIP("10.0.0.100")

	// Start in Master state
	sm.state = Master

	// Test handling packet with wrong VRID (should be ignored)
	wrongVRID := &Packet{
		VRID:     20,
		Priority: 200,
	}
	sm.handlePacket(wrongVRID)
	if sm.GetState() != Master {
		t.Error("Should remain Master when receiving packet with wrong VRID")
	}

	// Test priority 0 packet (should trigger advertisement)
	priorityZero := &Packet{
		VRID:     10,
		Priority: 0,
	}
	sm.handlePacket(priorityZero)
	// Check that event was sent (would need to read from channel in real test)

	// Test higher priority packet (should transition to Backup)
	higherPriority := &Packet{
		VRID:        10,
		Priority:    200,
		IPAddresses: []net.IP{net.ParseIP("10.0.0.50")},
	}
	sm.handlePacket(higherPriority)
	if sm.GetState() != Backup {
		t.Error("Should transition to Backup when receiving higher priority")
	}

	// Reset to Master for tie-break test
	sm.state = Master

	// Test same priority with lower source IP (we should win)
	samePriorityLower := &Packet{
		VRID:        10,
		Priority:    100,
		IPAddresses: []net.IP{net.ParseIP("10.0.0.50")},
	}
	sm.handlePacket(samePriorityLower)
	if sm.GetState() != Master {
		t.Error("Should remain Master when we win source IP tie-break")
	}

	// Test same priority with higher source IP (they should win)
	samePriorityHigher := &Packet{
		VRID:        10,
		Priority:    100,
		IPAddresses: []net.IP{net.ParseIP("10.0.0.200")},
	}
	sm.handlePacket(samePriorityHigher)
	if sm.GetState() != Backup {
		t.Error("Should transition to Backup when losing source IP tie-break")
	}
}

func TestByteComparison(t *testing.T) {
	// Test that our byte comparison works correctly
	ip1 := net.ParseIP("192.168.1.1").To4()
	ip2 := net.ParseIP("192.168.1.2").To4()
	ip3 := net.ParseIP("192.168.1.1").To4()

	if bytes.Compare(ip1, ip2) >= 0 {
		t.Error("192.168.1.1 should be less than 192.168.1.2")
	}

	if bytes.Compare(ip2, ip1) <= 0 {
		t.Error("192.168.1.2 should be greater than 192.168.1.1")
	}

	if !ip1.Equal(ip3) {
		t.Error("Same IPs should compare equal")
	}
}
