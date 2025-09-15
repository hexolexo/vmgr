# vmgr

Bubble Tea TUI for VM management and SSH access

## Features
- Automatically SSH to single running VM
- TUI selection interface for multiple VMs
- Configurable SSH usernames per VM

## Installation
1. Install Go
2. Clone repository
3. `go build` (or `go install` if you actually like whatever this is)

## Configuration
Create `~/.config/vmgr/vmgr.conf` with key-value pairs mapping VM names to SSH usernames:
```
vm-name=username
lab-server=root
windows-box=Administrator
```

## Usage
```
Mode: Toggle
> [ ] lab-mint-client
  [ ] dc-win-server
  [ ] web-alpine-client
Press q to quit.
```

## Requirements
- libvirt
- SSH installed and running on client VMs
