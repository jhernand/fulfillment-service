package main

import (
	"fmt"
	"log/slog"
	"os"

	"libvirt.org/go/libvirt"
)

// Example program that demonstrates how to use the libvirt Go bindings to connect to the libvirt daemon and create a
// virtual network.
//
// Usage:
//   go run example_libvirt.go
//
// Requirements:
//   - libvirt daemon must be running
//   - User must have permissions to connect to libvirt (usually via libvirt group membership)
//
// The program will:
//   1. Connect to the local libvirt daemon
//   2. Create a virtual network named "example-network"
//   3. Start the network
//   4. Display information about the network
//   5. Clean up by stopping and removing the network

func main() {
	// Create a logger that writes to stdout
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	logger.InfoContext(nil, "Starting libvirt example")

	// Connect to the libvirt daemon
	// Using "qemu:///system" for QEMU/KVM system connection
	// Other options include:
	//   - "qemu:///session" for user session
	//   - "lxc:///" for LXC containers
	//   - "xen:///" for Xen hypervisor
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to connect to libvirt",
			slog.Any("error", err),
		)
		os.Exit(1)
	}
	defer conn.Close()

	logger.InfoContext(nil, "Successfully connected to libvirt")

	// Get libvirt version information
	version, err := conn.GetVersion()
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to get libvirt version",
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	logger.InfoContext(
		nil,
		"Libvirt version information",
		slog.Uint64("version", uint64(version)),
	)

	// Get hypervisor information
	hypervisorVersion, err := conn.GetLibVersion()
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to get hypervisor version",
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	logger.InfoContext(
		nil,
		"Hypervisor version information",
		slog.Uint64("version", uint64(hypervisorVersion)),
	)

	// Define the network XML configuration
	// This creates a NAT network with DHCP on the 192.168.100.0/24 subnet
	networkXML := `
<network>
  <name>example-network</name>
  <bridge name="virbr-example"/>
  <forward mode="nat"/>
  <ip address="192.168.100.1" netmask="255.255.255.0">
    <dhcp>
      <range start="192.168.100.2" end="192.168.100.254"/>
    </dhcp>
  </ip>
</network>
`

	logger.InfoContext(
		nil,
		"Creating virtual network",
		slog.String("network", "example-network"),
	)

	// Create the network (transient, will not persist after reboot)
	network, err := conn.NetworkCreateXML(networkXML)
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to create network",
			slog.Any("error", err),
		)
		os.Exit(1)
	}
	defer func() {
		// Clean up: destroy the network when done
		if err := network.Destroy(); err != nil {
			logger.ErrorContext(
				nil,
				"Failed to destroy network",
				slog.Any("error", err),
			)
		} else {
			logger.InfoContext(nil, "Successfully destroyed network")
		}
		network.Free()
	}()

	logger.InfoContext(nil, "Successfully created network")

	// Get network information
	networkName, err := network.GetName()
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to get network name",
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	networkUUID, err := network.GetUUIDString()
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to get network UUID",
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	networkActive, err := network.IsActive()
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to check if network is active",
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	networkBridgeName, err := network.GetBridgeName()
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to get bridge name",
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	logger.InfoContext(
		nil,
		"Network information",
		slog.String("name", networkName),
		slog.String("uuid", networkUUID),
		slog.Bool("active", networkActive),
		slog.String("bridge", networkBridgeName),
	)

	// Get the network XML description
	xmlDesc, err := network.GetXMLDesc(0)
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to get network XML description",
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	logger.InfoContext(
		nil,
		"Network XML description",
		slog.String("xml", xmlDesc),
	)

	// List all networks
	networks, err := conn.ListAllNetworks(libvirt.CONNECT_LIST_NETWORKS_ACTIVE)
	if err != nil {
		logger.ErrorContext(
			nil,
			"Failed to list networks",
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	logger.InfoContext(
		nil,
		"Active networks",
		slog.Int("count", len(networks)),
	)

	for _, net := range networks {
		name, err := net.GetName()
		if err != nil {
			logger.ErrorContext(
				nil,
				"Failed to get network name",
				slog.Any("error", err),
			)
			continue
		}
		logger.InfoContext(
			nil,
			"Found network",
			slog.String("name", name),
		)
		net.Free()
	}

	logger.InfoContext(nil, "Example completed successfully")

	fmt.Println("\n=== Example Summary ===")
	fmt.Printf("Successfully created and managed virtual network '%s'\n", networkName)
	fmt.Printf("Network UUID: %s\n", networkUUID)
	fmt.Printf("Bridge name: %s\n", networkBridgeName)
	fmt.Printf("Network is active: %v\n", networkActive)
	fmt.Println("\nThe network has been automatically cleaned up.")
}
