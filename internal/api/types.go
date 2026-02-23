package api

// CheckRequest is sent by the client to ask which files the server needs.
type CheckRequest struct {
	Name  string          `json:"name"`
	Files []FileHashEntry `json:"files"`
}

// FileHashEntry holds the destination path and SHA256 hash of a local file.
type FileHashEntry struct {
	Dest string `json:"dest"`
	Hash string `json:"hash"`
}

// CheckResponse tells the client which destination paths need to be uploaded.
type CheckResponse struct {
	Upload []string `json:"upload"`
}

// Manifest is the JSON part of the multipart deploy request.
type Manifest struct {
	Name     string        `json:"name"`
	Files    []FileEntry   `json:"files"`
	Scripts  *ScriptsEntry `json:"scripts,omitempty"`
	Systemd  *SystemdEntry `json:"systemd,omitempty"`
	Hooks    *HooksEntry   `json:"hooks,omitempty"`
	Inventory *Inventory   `json:"inventory,omitempty"`
}

// FileEntry describes a single file to be placed on the server.
// If ArchivePath is empty, the file already exists on the server (delta skip).
type FileEntry struct {
	ArchivePath string `json:"archive_path"`
	Dest        string `json:"dest"`
	Mode        string `json:"mode"`
	Hash        string `json:"hash"`
}

// ScriptsEntry holds archive paths for server-side hook scripts.
type ScriptsEntry struct {
	ServerPreArchivePath  string `json:"server_pre_archive_path,omitempty"`
	ServerPostArchivePath string `json:"server_post_archive_path,omitempty"`
}

// SystemdEntry describes an optional systemd unit to install.
type SystemdEntry struct {
	UnitArchivePath string `json:"unit_archive_path"`
	UnitDest        string `json:"unit_dest"`
	Enable          bool   `json:"enable"`
	Restart         bool   `json:"restart"`
}

// HooksEntry holds the resolved script paths on the server (after extraction).
type HooksEntry struct {
	ServerPre  string `json:"server_pre,omitempty"`
	ServerPost string `json:"server_post,omitempty"`
}

// Inventory declares the desired system state on the CT.
type Inventory struct {
	Packages []string           `json:"packages,omitempty"`
	Services []InventoryService `json:"services,omitempty"`
	Users    []InventoryUser    `json:"users,omitempty"`
}

// InventoryService describes a systemd service to manage.
type InventoryService struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	State   string `json:"state"` // "started" or "stopped"
}

// InventoryUser describes a system user to ensure exists.
type InventoryUser struct {
	Name   string   `json:"name"`
	Home   string   `json:"home,omitempty"`
	Shell  string   `json:"shell,omitempty"`
	Groups []string `json:"groups,omitempty"`
}
