//go:build integration
// +build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// Check if running as root
	if os.Geteuid() != 0 {
		println("Integration tests must be run as root")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestMasterElection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vip := os.Getenv("TEST_VIP")
	if vip == "" {
		vip = "10.0.0.100"
	}

	// Start instance 1 with lower priority (backup)
	instance1 := NewVRRPInstance(t, "ns1", "veth1", 10, 100, vip)
	if err := instance1.Start(ctx); err != nil {
		t.Fatalf("Failed to start instance 1: %v", err)
	}
	defer instance1.Stop()

	// Start instance 2 with higher priority (should become master)
	instance2 := NewVRRPInstance(t, "ns2", "veth2", 10, 200, vip)
	if err := instance2.Start(ctx); err != nil {
		t.Fatalf("Failed to start instance 2: %v", err)
	}
	defer instance2.Stop()

	// Wait for election
	time.Sleep(5 * time.Second)

	// Check that instance2 is master
	state2, err := instance2.GetState()
	if err != nil {
		t.Fatalf("Failed to get state of instance 2: %v", err)
	}

	if state2 != "MASTER" {
		t.Errorf("Instance 2 should be MASTER, got %s", state2)
	}

	// Check that instance1 is backup
	state1, err := instance1.GetState()
	if err != nil {
		t.Fatalf("Failed to get state of instance 1: %v", err)
	}

	if state1 != "BACKUP" {
		t.Errorf("Instance 1 should be BACKUP, got %s", state1)
	}

	// Verify VIP is on instance2
	hasVIP, err := CheckVIPPresent("ns2", "veth2", vip)
	if err != nil {
		t.Fatalf("Failed to check VIP: %v", err)
	}

	if !hasVIP {
		t.Error("VIP should be present on master (ns2)")
	}
}

func TestFailover(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	vip := os.Getenv("TEST_VIP")
	if vip == "" {
		vip = "10.0.0.100"
	}

	// Start master instance
	master := NewVRRPInstance(t, "ns1", "veth1", 20, 200, vip)
	if err := master.Start(ctx); err != nil {
		t.Fatalf("Failed to start master: %v", err)
	}

	// Start backup instance
	backup := NewVRRPInstance(t, "ns2", "veth2", 20, 100, vip)
	if err := backup.Start(ctx); err != nil {
		t.Fatalf("Failed to start backup: %v", err)
	}
	defer backup.Stop()

	// Wait for initial election
	time.Sleep(5 * time.Second)

	// Verify master state
	masterState, err := master.GetState()
	if err != nil {
		t.Fatalf("Failed to get master state: %v", err)
	}

	if masterState != "MASTER" {
		t.Errorf("Initial master should be MASTER, got %s", masterState)
	}

	// Stop master to trigger failover
	t.Log("Stopping master to trigger failover...")
	if err := master.Stop(); err != nil {
		t.Fatalf("Failed to stop master: %v", err)
	}

	// Wait for failover (3 * advert_interval + skew_time)
	time.Sleep(10 * time.Second)

	// Check that backup became master
	backupState, err := backup.GetState()
	if err != nil {
		t.Fatalf("Failed to get backup state: %v", err)
	}

	if backupState != "MASTER" {
		t.Errorf("Backup should have become MASTER after failover, got %s", backupState)
	}

	// Verify VIP moved to new master
	hasVIP, err := CheckVIPPresent("ns2", "veth2", vip)
	if err != nil {
		t.Fatalf("Failed to check VIP after failover: %v", err)
	}

	if !hasVIP {
		t.Error("VIP should be present on new master (ns2) after failover")
	}
}

func TestPreemption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	vip := os.Getenv("TEST_VIP")
	if vip == "" {
		vip = "10.0.0.100"
	}

	// Start low priority instance (becomes initial master)
	lowPrio := NewVRRPInstance(t, "ns1", "veth1", 30, 100, vip)
	if err := lowPrio.Start(ctx); err != nil {
		t.Fatalf("Failed to start low priority instance: %v", err)
	}
	defer lowPrio.Stop()

	// Wait for it to become master
	time.Sleep(5 * time.Second)

	state, err := lowPrio.GetState()
	if err != nil {
		t.Fatalf("Failed to get initial state: %v", err)
	}

	if state != "MASTER" {
		t.Errorf("Low priority instance should be initial MASTER, got %s", state)
	}

	// Start high priority instance (should preempt)
	highPrio := NewVRRPInstance(t, "ns2", "veth2", 30, 200, vip)
	if err := highPrio.Start(ctx); err != nil {
		t.Fatalf("Failed to start high priority instance: %v", err)
	}
	defer highPrio.Stop()

	// Wait for preemption
	time.Sleep(5 * time.Second)

	// Check that high priority became master
	highState, err := highPrio.GetState()
	if err != nil {
		t.Fatalf("Failed to get high priority state: %v", err)
	}

	if highState != "MASTER" {
		t.Errorf("High priority instance should have preempted and become MASTER, got %s", highState)
	}

	// Check that low priority became backup
	lowState, err := lowPrio.GetState()
	if err != nil {
		t.Fatalf("Failed to get low priority state after preemption: %v", err)
	}

	if lowState != "BACKUP" {
		t.Errorf("Low priority instance should have become BACKUP after preemption, got %s", lowState)
	}
}

func TestMultipleVRIDs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start VRID 40 on ns1 (master)
	vrid40_ns1 := NewVRRPInstance(t, "ns1", "veth1", 40, 200, "10.0.0.101")
	if err := vrid40_ns1.Start(ctx); err != nil {
		t.Fatalf("Failed to start VRID 40 on ns1: %v", err)
	}
	defer vrid40_ns1.Stop()

	// Start VRID 41 on ns1 (backup)
	vrid41_ns1 := NewVRRPInstance(t, "ns1", "veth1", 41, 100, "10.0.0.102")
	if err := vrid41_ns1.Start(ctx); err != nil {
		t.Fatalf("Failed to start VRID 41 on ns1: %v", err)
	}
	defer vrid41_ns1.Stop()

	// Start VRID 40 on ns2 (backup)
	vrid40_ns2 := NewVRRPInstance(t, "ns2", "veth2", 40, 100, "10.0.0.101")
	if err := vrid40_ns2.Start(ctx); err != nil {
		t.Fatalf("Failed to start VRID 40 on ns2: %v", err)
	}
	defer vrid40_ns2.Stop()

	// Start VRID 41 on ns2 (master)
	vrid41_ns2 := NewVRRPInstance(t, "ns2", "veth2", 41, 200, "10.0.0.102")
	if err := vrid41_ns2.Start(ctx); err != nil {
		t.Fatalf("Failed to start VRID 41 on ns2: %v", err)
	}
	defer vrid41_ns2.Stop()

	// Wait for elections
	time.Sleep(5 * time.Second)

	// Verify VRID 40: ns1 master, ns2 backup
	state40_ns1, _ := vrid40_ns1.GetState()
	state40_ns2, _ := vrid40_ns2.GetState()

	if state40_ns1 != "MASTER" {
		t.Errorf("VRID 40: ns1 should be MASTER, got %s", state40_ns1)
	}
	if state40_ns2 != "BACKUP" {
		t.Errorf("VRID 40: ns2 should be BACKUP, got %s", state40_ns2)
	}

	// Verify VRID 41: ns1 backup, ns2 master
	state41_ns1, _ := vrid41_ns1.GetState()
	state41_ns2, _ := vrid41_ns2.GetState()

	if state41_ns1 != "BACKUP" {
		t.Errorf("VRID 41: ns1 should be BACKUP, got %s", state41_ns1)
	}
	if state41_ns2 != "MASTER" {
		t.Errorf("VRID 41: ns2 should be MASTER, got %s", state41_ns2)
	}
}
