//go:build integration
// +build integration

package integration

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type VRRPInstance struct {
	Namespace string
	Interface string
	VRID      uint8
	Priority  uint8
	VIP       string
	cmd       *exec.Cmd
	output    *bytes.Buffer
	t         *testing.T
}

func NewVRRPInstance(t *testing.T, ns string, iface string, vrid uint8, priority uint8, vip string) *VRRPInstance {
	return &VRRPInstance{
		Namespace: ns,
		Interface: iface,
		VRID:      vrid,
		Priority:  priority,
		VIP:       vip,
		t:         t,
		output:    &bytes.Buffer{},
	}
}

func (v *VRRPInstance) Start(ctx context.Context) error {
	vrrpBin := os.Getenv("VRRP_BIN")
	if vrrpBin == "" {
		vrrpBin = "../../../vrrp"
	}

	args := []string{
		"netns", "exec", v.Namespace,
		vrrpBin, "run",
		"--interface", v.Interface,
		"--vrid", fmt.Sprintf("%d", v.VRID),
		"--priority", fmt.Sprintf("%d", v.Priority),
		"--vips", v.VIP,
	}

	v.cmd = exec.CommandContext(ctx, "ip", args...)
	v.cmd.Stdout = v.output
	v.cmd.Stderr = v.output

	if err := v.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start VRRP instance: %w", err)
	}

	// Wait for startup
	time.Sleep(2 * time.Second)
	return nil
}

func (v *VRRPInstance) Stop() error {
	if v.cmd != nil && v.cmd.Process != nil {
		if err := v.cmd.Process.Kill(); err != nil {
			return err
		}
		v.cmd.Wait()
	}
	return nil
}

func (v *VRRPInstance) GetState() (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(v.output.Bytes()))
	var lastState string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Current state:") {
			parts := strings.Split(line, "Current state:")
			if len(parts) > 1 {
				lastState = strings.TrimSpace(parts[1])
			}
		}
	}

	if lastState == "" {
		return "", fmt.Errorf("state not found in output")
	}

	return lastState, nil
}

func (v *VRRPInstance) WaitForState(expectedState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		state, err := v.GetState()
		if err == nil && strings.EqualFold(state, expectedState) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for state %s", expectedState)
}

func CheckVIPPresent(namespace, iface, vip string) (bool, error) {
	cmd := exec.Command("ip", "netns", "exec", namespace, "ip", "addr", "show", iface)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return strings.Contains(string(output), vip), nil
}

func RunCommand(namespace string, command ...string) (string, error) {
	args := []string{"netns", "exec", namespace}
	args = append(args, command...)

	cmd := exec.Command("ip", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func PingVIP(namespace, vip string) error {
	_, err := RunCommand(namespace, "ping", "-c", "1", "-W", "1", vip)
	return err
}

func WaitForVIP(namespace, iface, vip string, present bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		hasVIP, err := CheckVIPPresent(namespace, iface, vip)
		if err != nil {
			return err
		}

		if hasVIP == present {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	if present {
		return fmt.Errorf("timeout waiting for VIP %s to appear", vip)
	}
	return fmt.Errorf("timeout waiting for VIP %s to disappear", vip)
}

func CapturePackets(namespace, iface string, duration time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", namespace,
		"tcpdump", "-i", iface, "-w", "-", "proto", "112")

	output, _ := cmd.Output()
	return output, nil
}
