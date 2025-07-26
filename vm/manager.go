package vm

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/digitalocean/go-libvirt"
	"golang.org/x/term"
)

// VMManager handles all VM operations
type VMManager struct {
	conn *libvirt.Libvirt
}

// VMState represents the current state of VMs
type VMState struct {
	ActiveDomains   []libvirt.Domain
	InactiveDomains []libvirt.Domain
	AllDomains      []libvirt.Domain
	ActiveVMs       map[string]bool
}

// VMAction represents actions that can be performed on VMs
type VMAction int

const (
	Toggle VMAction = iota
	SSH
	VirtViewer
)

// NewVMManager creates a new VM manager instance
func NewVMManager() (*VMManager, error) {
	// Initialize libvirtd service
	if err := initLibvirtd(); err != nil {
		return nil, fmt.Errorf("failed to initialize libvirtd: %w", err)
	}

	// Connect to libvirt
	uri, _ := url.Parse(string(libvirt.QEMUSystem))
	conn, err := libvirt.ConnectToURI(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to libvirt: %w", err)
	}

	vm := &VMManager{conn: conn}

	// Initialize network
	if err := vm.initNetwork(); err != nil {
		conn.ConnectClose()
		return nil, fmt.Errorf("failed to initialize network: %w", err)
	}

	return vm, nil
}

// Close closes the libvirt connection
func (vm *VMManager) Close() {
	if vm.conn != nil {
		vm.conn.ConnectClose()
	}
}

// GetVMState retrieves current state of all VMs
func (vm *VMManager) GetVMState() (*VMState, error) {
	activeFlags := libvirt.ConnectListDomainsActive
	inactiveFlags := libvirt.ConnectListDomainsInactive

	activeDomains, _, err := vm.conn.ConnectListAllDomains(1, activeFlags)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve active domains: %w", err)
	}

	inactiveDomains, _, err := vm.conn.ConnectListAllDomains(1, inactiveFlags)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve inactive domains: %w", err)
	}
	domainSortFunc := func(domains []libvirt.Domain) {
		sort.Slice(domains, func(i, j int) bool {
			return strings.ToLower(domains[i].Name) < strings.ToLower(domains[j].Name)
		})
	}

	domainSortFunc(activeDomains)
	domainSortFunc(inactiveDomains)

	allDomains := append(inactiveDomains, activeDomains...)
	activeVMs := make(map[string]bool)
	domainSortFunc(allDomains)

	for _, domain := range activeDomains {
		activeVMs[domain.Name] = true
	}

	return &VMState{
		ActiveDomains:   activeDomains,
		InactiveDomains: inactiveDomains,
		AllDomains:      allDomains,
		ActiveVMs:       activeVMs,
	}, nil
}

// ExecuteAction performs the specified action on a domain
func (vm *VMManager) ExecuteAction(domain libvirt.Domain, action VMAction) error {
	switch action {
	case Toggle:
		state, _, err := vm.conn.DomainGetState(domain, 0)
		if err != nil {
			return fmt.Errorf("failed to get domain state: %w", err)
		}

		if state == 1 {
			return vm.stopDomain(domain)
		} else {
			return vm.startDomain(domain)
		}
	case SSH:
		return vm.connectSSH(domain)
	case VirtViewer:
		return vm.connectVirtViewer(domain)
	default:
		return fmt.Errorf("unknown action: %d", action)
	}
}

// startDomain starts a VM domain
func (vm *VMManager) startDomain(domain libvirt.Domain) error {
	return vm.conn.DomainCreate(domain)
}

// stopDomain stops a VM domain
func (vm *VMManager) stopDomain(domain libvirt.Domain) error {
	return vm.conn.DomainShutdown(domain)
}

// connectSSH establishes SSH connection to a domain
func (vm *VMManager) connectSSH(domain libvirt.Domain) error {
	// Get domain IP address
	domainInterface, err := vm.conn.DomainInterfaceAddresses(domain, 0, 0)
	if err != nil {
		return fmt.Errorf("could not get IP address: %w", err)
	}

	var ip string
	if len(domainInterface) >= 1 && len(domainInterface[0].Addrs) >= 1 {
		ip = domainInterface[0].Addrs[0].Addr
	} else {
		fmt.Errorf("domain is still booting or does not have a network adapter")
	}

	// Get remote user from config
	remoteUser, err := vm.getRemoteUser(domain.Name)
	if err != nil {
		return fmt.Errorf("error getting remote user: %w", err)
	}

	// Execute SSH connection
	return vm.executeSSH(remoteUser + "@" + ip)
}

// connectVirtViewer opens domain with virt-viewer
func (vm *VMManager) connectVirtViewer(domain libvirt.Domain) error {
	return fmt.Errorf("virt-viewer not implemented") //  FIX: Probably should just get rid of this
}

// executeSSH handles the SSH connection with PTY
func (vm *VMManager) executeSSH(target string) error {
	command := exec.Command("ssh", target)

	ptmx, err := pty.Start(command)
	if err != nil {
		return fmt.Errorf("failed to run SSH command: %w", err)
	}
	defer ptmx.Close()

	// Handle pty size
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("error resizing pty: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH
	defer func() { signal.Stop(ch); close(ch) }()

	// Set stdin in raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Copy stdin to pty and pty to stdout
	go func() { io.Copy(ptmx, os.Stdin) }()
	io.Copy(os.Stdout, ptmx)

	return nil
}

// getRemoteUser reads the remote user from config file
func (vm *VMManager) getRemoteUser(domainName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".config/vmgr/vmgr.conf")
	return readConfigFile(configPath, domainName)
}

// initLibvirtd initializes the libvirtd service
func initLibvirtd() error {
	_, err := exec.Command("systemctl", "is-active", "libvirtd").Output()
	if err != nil {
		cmd := exec.Command("systemctl", "start", "libvirtd")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start libvirtd: %w", err)
		}
		fmt.Println("libvirtd service started successfully")
	}
	return nil
}

// initNetwork initializes the default virtual network
func (vm *VMManager) initNetwork() error {
	// Get all defined networks
	networks, _, err := vm.conn.ConnectListAllNetworks(1, 0) // flags=1 for all networks
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	var errors []string
	for _, network := range networks {
		// Try to start each network
		err = vm.conn.NetworkCreate(network)
		if err != nil && !strings.Contains(err.Error(), "network is already active") {
			errors = append(errors, fmt.Sprintf("failed to start network '%s': %v", network.Name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("network initialization errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// readConfigFile reads a key-value config file
func readConfigFile(filePath, key string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		// If config file doesn't exist, return current user
		usr, userErr := user.Current()
		if userErr != nil {
			return "", userErr
		}
		return usr.Username, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if k == key {
			return v, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	// If key not found, return current user
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return usr.Username, nil
}

// ActionString returns string representation of VMAction
func (a VMAction) String() string {
	switch a {
	case Toggle:
		return "Toggle"
	case SSH:
		return "SSH"
	case VirtViewer:
		return "VirtViewer"
	default:
		return "Unknown"
	}
}

// GetActionPrompt returns the prompt string for an action
func (a VMAction) GetActionPrompt() string {
	switch a {
	case Toggle:
		return "Toggle\n\n"
	case SSH:
		return "Open with SSH\n\n"
	case VirtViewer:
		return "Open with virt-viewer\n\n"
	default:
		return "An error has occurred"
	}
}
