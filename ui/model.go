package ui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/digitalocean/go-libvirt"

	"vmgr/vm"
)

type model struct {
	cursor      int
	domainToSSH *libvirt.Domain
	manager     vm.VMManager
	state       *vm.VMState
	mode        int
	err         error
}

func StartTUI(manager vm.VMManager, state vm.VMState) (*libvirt.Domain, error) {
	var err error
	var domainToSSH *libvirt.Domain
	p := tea.NewProgram(initialModel(manager, state))
	finalModel, err := p.Run() //  BUG: PANIC invalid memory address or nil pointer dereference

	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		return nil, err
	}

	if m, ok := finalModel.(model); ok {
		err = m.err
		domainToSSH = m.domainToSSH // This is returning nil
	}
	return domainToSSH, err
}
func initialModel(manager vm.VMManager, state vm.VMState) model {
	return model{
		manager:     manager,
		state:       &state,
		domainToSSH: nil,
		err:         nil,
	}
}

func (m model) Init() tea.Cmd {
	return tea.SetWindowTitle("vmgr")
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var err error
	m.state, err = m.manager.GetVMState()
	if err != nil {
		return m, tea.Quit
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.state.AllDomains)-1 {
				m.cursor++
			}
		case "left", "right":
			m.mode ^= 1 // Flips from 0 and 1  TODO: Should later add more options
		case "enter", " ":
			return vmAction(m)
		}
	}

	return m, nil
}

func (m model) View() string {
	s := "VM management\n\nMode: "
	switch m.mode {
	case 0:
		s += "Toggle\n"
	case 1:
		s += "SSH\n"
	}

	for i, domain := range m.state.AllDomains {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		checked := " "
		if slices.Contains(m.state.ActiveDomains, domain) {
			checked = "x"
		}

		s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, domain.Name)
	}

	s += "\nPress q to quit.\n"

	return s
}

func vmAction(m model) (tea.Model, tea.Cmd) {
	selectedVM := m.state.AllDomains[m.cursor]
	switch m.mode {
	case 0:
		m.manager.ExecuteAction(selectedVM, vm.Toggle)
		return m, nil
	case 1:
		if !slices.Contains(m.state.ActiveDomains, m.state.AllDomains[m.cursor]) { //  HACK: Honestly this whole section needs to be rewritten eventually
			_ = m.manager.ExecuteAction(selectedVM, vm.Toggle) // Will only ever turn on Also should probably get some error handling
		}
		m.domainToSSH = &selectedVM
		return m, tea.Quit
	default:
		fmt.Println("Invalid action")
		return m, nil
	}
}
