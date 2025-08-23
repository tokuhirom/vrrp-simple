package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"

	"github.com/tokuhirom/vrrp-simple/pkg/vrrp"
)

var (
	app = kingpin.New("vrrp", "Simple VRRP implementation")

	runCmd       = app.Command("run", "Run VRRP instance")
	runInterface = runCmd.Flag("interface", "Network interface to use").Short('i').Required().String()
	runVRID      = runCmd.Flag("vrid", "Virtual Router ID (1-255)").Short('r').Required().Uint8()
	runPriority  = runCmd.Flag("priority", "Router priority (1-255, 255 = master)").Short('p').Default("100").Uint8()
	runVIPs      = runCmd.Flag("vips", "Virtual IP addresses (comma-separated)").Short('v').Required().String()
	runInterval  = runCmd.Flag("advert-int", "Advertisement interval in seconds").Default("1").Int()
	runPreempt   = runCmd.Flag("preempt", "Enable preemption").Default("true").Bool()

	statusCmd       = app.Command("status", "Show VRRP status")
	statusInterface = statusCmd.Flag("interface", "Network interface").Short('i').String()
	statusVRID      = statusCmd.Flag("vrid", "Virtual Router ID").Short('r').Uint8()

	versionCmd = app.Command("version", "Show version information")
)

const Version = "0.1.0"

func main() {
	app.HelpFlag.Short('h')
	app.Version(Version)

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case runCmd.FullCommand():
		runVRRP()
	case statusCmd.FullCommand():
		showStatus()
	case versionCmd.FullCommand():
		showVersion()
	}
}

func runVRRP() {
	vips := strings.Split(*runVIPs, ",")
	for i, vip := range vips {
		vips[i] = strings.TrimSpace(vip)
	}

	config := &vrrp.Config{
		VRID:        *runVRID,
		Priority:    *runPriority,
		Interface:   *runInterface,
		VirtualIPs:  vips,
		AdvInterval: *runInterval,
		Preempt:     *runPreempt,
		Version:     vrrp.VRRPv2,
	}

	router, err := vrrp.NewVirtualRouter(config)
	if err != nil {
		log.Fatalf("Failed to create virtual router: %v", err)
	}

	if err := router.Start(); err != nil {
		log.Fatalf("Failed to start virtual router: %v", err)
	}

	fmt.Printf("VRRP started:\n")
	fmt.Printf("  Interface: %s\n", *runInterface)
	fmt.Printf("  VRID: %d\n", *runVRID)
	fmt.Printf("  Priority: %d\n", *runPriority)
	fmt.Printf("  Virtual IPs: %s\n", strings.Join(vips, ", "))
	fmt.Printf("  Advertisement Interval: %d seconds\n", *runInterval)
	fmt.Printf("  Preemption: %v\n", *runPreempt)
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state := router.GetState()
				fmt.Printf("[%s] VRID %d: Current state: %s\n",
					time.Now().Format("15:04:05"),
					router.GetVRID(),
					state)
			}
		}
	}()

	sig := <-sigCh
	fmt.Printf("\nReceived signal %v, shutting down...\n", sig)

	if err := router.Stop(); err != nil {
		log.Printf("Error stopping router: %v", err)
	}

	fmt.Println("VRRP stopped")
}

func showStatus() {
	fmt.Println("Status command implementation")
	fmt.Println("This would show the current status of VRRP instances")

	if *statusInterface != "" {
		fmt.Printf("Interface filter: %s\n", *statusInterface)
	}
	if *statusVRID != 0 {
		fmt.Printf("VRID filter: %d\n", *statusVRID)
	}

	fmt.Println("\nNote: Full status implementation requires IPC or shared state mechanism")
}

func showVersion() {
	fmt.Printf("vrrp-simple version %s\n", Version)
	fmt.Println("A simple VRRP implementation in Go")
	fmt.Println("https://github.com/tokuhirom/vrrp-simple")
}
