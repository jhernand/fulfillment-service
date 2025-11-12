# Libvirt Go Example

This example program demonstrates how to use the [libvirt Go bindings](https://pkg.go.dev/libvirt.org/go/libvirt) to connect to the libvirt daemon and create a virtual network.

## Prerequisites

### 1. Install libvirt

On Fedora/RHEL:

```bash
sudo dnf install libvirt libvirt-devel
```

On Ubuntu/Debian:

```bash
sudo apt-get install libvirt-daemon libvirt-dev
```

### 2. Start the libvirt daemon

```bash
sudo systemctl start libvirtd
sudo systemctl enable libvirtd
```

### 3. Add your user to the libvirt group

To connect to the system libvirt instance without root privileges:

```bash
sudo usermod -a -G libvirt $USER
```

Then log out and log back in for the group membership to take effect.

## Running the Example

From the project root directory:

```bash
cd examples/libvirt
go run main.go
```

Or build and run:

```bash
cd examples/libvirt
go build -o libvirt-example
./libvirt-example
```

## What the Example Does

The program performs the following operations:

1. **Connects to libvirt**: Establishes a connection to the local QEMU/KVM system instance (`qemu:///system`)

2. **Gets version information**: Retrieves and displays the libvirt and hypervisor versions

3. **Creates a virtual network**: Creates a transient virtual network named `example-network` with:
   - NAT forwarding mode
   - Bridge interface `virbr-example`
   - IP range: `192.168.100.0/24`
   - DHCP range: `192.168.100.2` - `192.168.100.254`
   - Gateway: `192.168.100.1`

4. **Displays network information**: Shows the network name, UUID, active status, and bridge name

5. **Lists all active networks**: Enumerates all currently active networks

6. **Cleans up**: Automatically destroys the network when the program exits

## Network Configuration

The example creates a NAT network with the following XML configuration:

```xml
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
```

This configuration creates a NAT network that allows virtual machines to access external networks through the host, similar to how a home router works.

## Customization

You can modify the `networkXML` variable in the code to customize:

- Network name
- Bridge name
- IP address range
- DHCP configuration
- Forwarding mode (NAT, routed, isolated, bridge, etc.)

## Troubleshooting

### Permission denied errors

If you get permission errors when running the example, make sure:
- You're a member of the `libvirt` group
- The libvirt daemon is running
- You've logged out and back in after adding yourself to the libvirt group

### Connection refused

If you can't connect to libvirt:
- Check that the libvirt daemon is running: `sudo systemctl status libvirtd`
- Try connecting as root: `sudo go run example_libvirt.go`

### Network already exists

If you get an error that the network already exists:
- List existing networks: `virsh net-list --all`
- Remove the conflicting network: `virsh net-destroy example-network` (if active)
- The example uses a transient network that should be automatically cleaned up

## Additional Resources

- [libvirt Go bindings documentation](https://pkg.go.dev/libvirt.org/go/libvirt)
- [libvirt networking documentation](https://libvirt.org/formatnetwork.html)
- [libvirt Go project on GitLab](https://gitlab.com/libvirt/libvirt-go-module)
