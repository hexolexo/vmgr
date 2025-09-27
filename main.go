package main

import (
	"fmt"
	"log"
	"os"
	"time"
	"vmgr/ui"
	"vmgr/vm"

	"github.com/digitalocean/go-libvirt"
)

func main() {
	os.Exit(run()) // Not sure if I'm a fan of this pattern
}

func run() int {
	vmMgr, err := vm.NewVMManager()
	if err != nil {
		log.Fatal("Failed to create VM manager:", err)
	}
	defer vmMgr.Close() //  WARN: This does not close if the program exits with os.Exit(0)

	shouldExit := false
	var domainToSSH *libvirt.Domain
	domainToSSH = nil // Totally secure programming practices /j
	for {
		state, err := vmMgr.GetVMState()

		if err != nil {
			log.Printf("Failed to get VM state: %s", err)
			return 1
		}

		if domainToSSH == nil && len(state.ActiveDomains) == 1 {
			domainToSSH = &state.ActiveDomains[0] // If an active domain exists set it as target
		}

		if domainToSSH != nil {
			shouldExit = sshToVM(vmMgr, *domainToSSH) // If a target is available SSH into it
		}

		if shouldExit {
			return 0
		}

		domainToSSH, err = ui.StartTUI(*vmMgr, *state)
		if err != nil {
			log.Printf("Bubble Tea had an error: %v", err)
			return 1
		}

		if domainToSSH == nil { // Leave me and my ratsnest of if statements alone
			return 0
		}

	}
}
func sshToVM(vmMgr *vm.VMManager, domain libvirt.Domain) bool {
	sshConnectionStarted := time.Now()

	fmt.Printf("Connecting to VM: %s via SSH\n", domain.Name)
	maxAttempts := 3
	attempt := 0

retry:
	attempt++
	err := vmMgr.ExecuteAction(domain, vm.SSH)
	if err != nil {
		log.Printf("Failed to SSH into VM %s: %v", domain.Name, err)
		if attempt < maxAttempts {
			time.Sleep(time.Second * 2)
			goto retry
		}
		return false
	}

	if time.Since(sshConnectionStarted) > 5*time.Second {
		return true
	}
	return false //  HACK: Magic variable
}
