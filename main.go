package main

import (
	"fmt"
	"log"
	"time"
	"vmgr/ui"
	"vmgr/vm"

	"github.com/digitalocean/go-libvirt"
)

func main() {
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
			vmMgr.Close()                             //  HACK: Might want to find a more elegant solution
			log.Fatal("Failed to get VM state:", err) //  HACK: The nesting might also be an issue
		}
		if len(state.ActiveDomains) == 1 && domainToSSH == nil {
			shouldExit = sshToVM(*vmMgr, state.ActiveDomains[0])
		} else if domainToSSH != nil {
			shouldExit = sshToVM(*vmMgr, *domainToSSH)
		}
		if shouldExit {
			return
		}
		domainToSSH, err = ui.StartTUI(*vmMgr, *state)
		if err != nil {
			log.Printf("Bubble Tea had an error: %v", err)
			return
		}
		if domainToSSH == nil { // Leave me and my ratsnest of if statements alone
			return
		}
	}
}
func sshToVM(vmMgr vm.VMManager, domain libvirt.Domain) bool {
	sshConnectionStarted := time.Now()

	fmt.Printf("Connecting to VM: %s via SSH\n", domain.Name)

	err := vmMgr.ExecuteAction(domain, vm.SSH) // Should put something here for repetition if it fails
	if err != nil {
		log.Printf("Failed to SSH into VM %s: %v", domain.Name, err)
		return false
	}

	if time.Since(sshConnectionStarted) > 5*time.Second {
		return true
	}
	return false //  HACK: Magic varible
}
