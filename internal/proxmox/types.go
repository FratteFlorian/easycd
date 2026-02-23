package proxmox

// LXCCreateConfig holds parameters for creating an LXC container.
type LXCCreateConfig struct {
	VMID          int
	Node          string
	Hostname      string
	Template      string // e.g. "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
	Storage       string // e.g. "local-lvm"
	Cores         int
	Memory        int    // MB
	Swap          int    // MB
	DiskGB        int    // rootfs size in GB
	Net0          string // e.g. "name=eth0,bridge=vmbr0,firewall=1,ip=dhcp"
	Password      string // root password (optional if SSHPublicKeys is set)
	SSHPublicKeys string // injected into /root/.ssh/authorized_keys
}

// TaskStatus represents the status of an async Proxmox task.
type TaskStatus struct {
	UPID       string `json:"upid"`
	Status     string `json:"status"`     // "running" | "stopped"
	ExitStatus string `json:"exitstatus"` // "OK" or error message
	Type       string `json:"type"`
}

// Interface represents a network interface on an LXC container.
type Interface struct {
	Name    string `json:"name"`
	Inet    string `json:"inet"`  // IPv4 address with CIDR, e.g. "192.168.1.100/24"
	Inet6   string `json:"inet6"`
	HwAddr  string `json:"hwaddr"`
}

// StorageInfo represents a Proxmox storage backend.
type StorageInfo struct {
	Storage string `json:"storage"`
	Type    string `json:"type"`
	Active  int    `json:"active"`
	Content string `json:"content"` // comma-separated content types
}

// Template represents an available LXC OS template.
type Template struct {
	VolID    string `json:"volid"`    // e.g. "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
	Content  string `json:"content"`  // "vztmpl"
	Size     int64  `json:"size"`
}

// apiResponse wraps the Proxmox API JSON envelope.
type apiResponse struct {
	Data interface{} `json:"data"`
}
