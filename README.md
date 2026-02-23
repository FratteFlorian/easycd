# simplecd

A lightweight, opinionated continuous deployment tool for self-hosted projects.
Push your build artifacts directly onto a Proxmox LXC container — no CI/CD platform, no Kubernetes, no cloud required.

```
simplecd deploy
[simplecd] Files to upload: 3 / 42
[simplecd] Deploying my-api → http://192.168.1.50:8765
[simplecd] Installing packages: [nginx]
[simplecd] rollback: backing up 3 files
[simplecd] Placing /usr/local/bin/my-api
[simplecd] Deployment complete
```

---

## How it works

simplecd consists of two small binaries:

| Binary | Role |
|---|---|
| `simplecd` | Client CLI — runs on your dev machine |
| `simplecdd` | Daemon — runs on the target LXC container |

The client computes SHA-256 hashes of your build output, asks the server which files actually changed, and uploads only the delta as a compressed archive. The server extracts it, reconciles system state (packages, services, users), and runs your hooks — all streamed back to your terminal in real time.

---

## Features

- **Delta uploads** — only changed files are transferred
- **Proxmox-native** — optional one-command LXC provisioning with `simplecd init`
- **Inventory management** — declaratively install packages, manage systemd services, and create users
- **Rollback** — automatic pre-deploy backup; restore with `simplecd rollback`
- **Systemd integration** — install, enable, and restart units as part of the deploy
- **Hooks** — local pre-build, server pre-deploy, and server post-deploy scripts
- **No dependencies** — stdlib + one YAML library; no Docker, no agent framework

---

## Quick start

### Option A — Proxmox wizard (recommended)

Provision a fresh LXC container and install `simplecdd` automatically:

```sh
# Build both binaries first
make build

# Run the interactive wizard
./dist/simplecd init
# → "Create a new LXC container on Proxmox?" → Yes
# Wizard prompts for Proxmox credentials, container parameters,
# then creates the CT, bootstraps simplecdd, and pre-fills your config.
```

The wizard:
1. Connects to your Proxmox node (credentials saved to `~/.config/simplecd/proxmox.yaml`)
2. Creates an unprivileged LXC container from a template you choose
3. Installs `simplecdd` via SSH, generates a random auth token, enables the systemd unit
4. Writes `server:` and `token:` into `.simplecd/config.yaml` automatically

### Option B — Existing server

Install `simplecdd` manually on any Linux host:

```sh
make build

# Copy the binary and service file
scp dist/simplecdd root@<host>:/usr/local/bin/simplecdd
scp install/simplecdd.service root@<host>:/etc/systemd/system/

# Configure the daemon
ssh root@<host> "
  mkdir -p /etc/simplecd /var/log/simplecd /var/lib/simplecd
  cat > /etc/simplecd/server.yaml <<'EOF'
listen: :8765
token: $(openssl rand -hex 32)
log_dir: /var/log/simplecd
EOF
  systemctl daemon-reload
  systemctl enable --now simplecdd
"

# Then initialise your project locally
./dist/simplecd init
# → "Create a new LXC container on Proxmox?" → No
```

---

## Project configuration

`simplecd init` creates `.simplecd/config.yaml` in your project root.
`.simplecd/` is automatically added to `.gitignore`.

```yaml
name: my-api
server: http://192.168.1.50:8765
# token: keep-this-in-SIMPLECD_TOKEN-env-var

deploy:
  mappings:
    - src: ./dist          # relative to project root
      dest: /usr/local/bin # absolute path on the CT
      mode: "0755"
      dir_mode: "0755"
      exclude:
        - "*.log"
        - ".git/"

  # Optional: install/reload a systemd unit on every deploy
  systemd:
    unit: .simplecd/my-api.service
    enable: true
    restart: true

hooks:
  local_pre:   .simplecd/local-pre.sh   # runs on your machine before upload
  server_pre:  .simplecd/stop.sh        # runs on the CT before files are placed
  server_post: .simplecd/start.sh       # runs on the CT after files are placed
```

**Token resolution order:** `SIMPLECD_TOKEN` env var → `token:` field in config.

Multiple `mappings` are supported — useful when you deploy a binary, a config file, and a static directory to different locations in one shot.

---

## Inventory

Create `.simplecd/inventory.yaml` to declare the system state that must exist on the CT before your files land. simplecd diffs the desired state against the previous deploy and only acts on changes.

```yaml
packages:
  - nginx
  - postgresql-client

services:
  - name: nginx
    enabled: true
    state: started
  - name: my-old-service
    enabled: false
    state: stopped

users:
  - name: appuser
    home: /home/appuser
    shell: /bin/bash
    groups:
      - www-data
```

**Package ownership tracking** — if two projects both declare `curl`, it won't be removed when one of them drops it. Ownership state is stored at `/var/lib/simplecd/.global/package-owners.json`.

Supported package managers: `apt-get`, `dnf`, `yum`, `pacman`.

---

## Rollback

Every deploy automatically snapshots the files it is about to overwrite.
To restore the previous version:

```sh
simplecd rollback
[simplecd] Rolling back my-api...
[simplecd] rollback: restoring /usr/local/bin/my-api
[simplecd] Rollback complete
```

The snapshot is stored at `/var/lib/simplecd/<project>/rollback/` on the CT.
Only one snapshot (the most recent deploy) is kept per project.

---

## Commands

```
simplecd init [--reinit]   Interactive wizard — creates .simplecd/config.yaml
simplecd deploy            Deploy to the configured server
simplecd rollback          Restore the previous deployment snapshot
```

| Flag | Command | Default | Description |
|---|---|---|---|
| `--reinit` / `-r` | `init` | false | Overwrite existing config |
| `--dir <path>` | `deploy`, `rollback` | `.` | Project directory |

---

## Server daemon

`simplecdd` exposes a small HTTP API on the CT:

| Endpoint | Method | Description |
|---|---|---|
| `/check` | POST | Return which files differ from the client's hashes |
| `/deploy` | POST | Receive and apply a deployment |
| `/rollback` | POST | Restore the previous snapshot |
| `/health` | GET | Liveness probe (no auth required) |

Rate limits: `/check` — 60 req/min per IP; `/deploy`, `/rollback` — 10 req/min per IP.
Deployments are serialized (one at a time).

**Server config** (`/etc/simplecd/server.yaml`):

```yaml
listen: :8765
token: <32+ char random string>
log_dir: /var/log/simplecd
```

Logs are written to `<log_dir>/simplecdd.log` and to stdout.

---

## Proxmox config

Proxmox credentials are stored at `~/.config/simplecd/proxmox.yaml` and reused across projects:

```yaml
host: 192.168.1.10
port: 8006
node: pve
token: root@pam!mytoken=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
insecure: true   # skip TLS verification for self-signed certs
```

The `PROXMOX_TOKEN` environment variable overrides the `token:` field.

Required Proxmox API token permissions: `VM.Allocate`, `VM.Config.*`, `Datastore.AllocateSpace`, `SDN.Use` (or equivalent on the target pool/node).

---

## Build

```sh
# Both binaries
make build

# Client only (runs on your machine — any OS)
make build-client

# Server binary only (Linux/amd64 — runs on the CT)
make build-server

# Run tests
make test
```

Output goes to `dist/`.

---

## State on the CT

| Path | Contents |
|---|---|
| `/etc/simplecd/server.yaml` | Daemon config |
| `/var/log/simplecd/simplecdd.log` | Deploy logs |
| `/var/lib/simplecd/<project>/rollback/` | Pre-deploy file snapshot |
| `/var/lib/simplecd/<project>/inventory.json` | Last-applied inventory state |
| `/var/lib/simplecd/.global/package-owners.json` | Cross-project package ownership |

---

## Using simplecd with a public VPS

The Proxmox wizard is Proxmox-specific, but `simplecdd` runs on any Linux host. If your target is a public VPS (DigitalOcean, Hetzner, Contabo, …), **don't expose port 8765 to the internet**. Use one of these approaches instead:

**Option 1 — SSH tunnel**

Forward port 8765 over SSH before deploying. No firewall changes needed.

```sh
# Open the tunnel in the background
ssh -L 8765:localhost:8765 root@vps.example.com -N &
TUNNEL_PID=$!

# Point your config at localhost
# server: http://localhost:8765

simplecd deploy

kill $TUNNEL_PID
```

**Option 2 — VPN (Tailscale / WireGuard)**

Add both your dev machine and the VPS to the same VPN. The VPS gets a private VPN IP and simplecd behaves exactly like on a LAN — no extra steps per deploy.

```sh
# Install Tailscale on the VPS
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up

# Use the Tailscale IP in your config
# server: http://100.x.y.z:8765
```

---

## Philosophy

- **One CT per project.** Each deployment target is an isolated LXC container — no shared state between projects.
- **Opinionated, not extensible.** simplecd does one thing: get your build output onto a container and keep it running. For anything more complex, reach for Ansible or NixOS.
- **No inbound ports.** The target only needs to be reachable from your dev machine on port 8765. Expose your application port via Cloudflare Tunnel or a reverse proxy — simplecd itself does not require internet access.

---

## License

MIT
