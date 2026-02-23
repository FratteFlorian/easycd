package cmd

// stackTemplate describes a predefined inventory + mapping guide for a tech stack.
type stackTemplate struct {
	label         string
	suggestedSrc  string // default src mapping (overridden if user has a build step)
	suggestedDest string // suggested dest prefix (appended with project name)
	mappingHint   string // printed before the dest-dir question; <name> is replaced
	inventoryYAML string // written to .simplecd/inventory.yaml
}

// stackTemplates holds the available presets keyed by a short identifier.
var stackTemplates = map[string]stackTemplate{
	"nodejs": {
		label:         "Node.js",
		suggestedSrc:  "./",
		suggestedDest: "/var/www",
		mappingHint: `  src:  ./              → /var/www/<name>/   (whole project directory)
  mode: "0644"
  exclude: node_modules/, .env

  Runtime: node /var/www/<name>/index.js   (or use PM2)
  Tip: nginx reverse-proxies port 80 → your app port (e.g. 3000).
       Add PM2 to packages for production process management.
  Hooks:
    server_pre:  pm2 stop <name> || true
    server_post: pm2 start /var/www/<name>/index.js --name <name>`,
		inventoryYAML: `packages:
  - nodejs
  - npm
  # - nginx   # uncomment to add a reverse proxy

# services:
#   - name: nginx
#     enabled: true
#     state: started
`,
	},

	"python": {
		label:         "Python",
		suggestedSrc:  "./",
		suggestedDest: "/opt",
		mappingHint: `  src:  ./              → /opt/<name>/
  mode: "0644"
  exclude: __pycache__/, .venv/, venv/, *.pyc

  Runtime: /opt/<name>/.venv/bin/python app.py   (or gunicorn/uvicorn)
  Tip: install dependencies in the server_post hook:
       cd /opt/<name> && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt`,
		inventoryYAML: `packages:
  - python3
  - python3-pip
  - python3-venv
  # - nginx     # uncomment to add a reverse proxy
`,
	},

	"laravel": {
		label:         "Laravel (PHP)",
		suggestedSrc:  "./",
		suggestedDest: "/var/www",
		mappingHint: `  src:  ./              → /var/www/<name>/
  mode: "0644"
  exclude: vendor/, node_modules/, .env

  Webroot (nginx/apache): /var/www/<name>/public
  WARNING: never deploy .env — place it manually on the CT.
  Tip: run composer in server_post hook:
       cd /var/www/<name> && composer install --no-dev --optimize-autoloader`,
		inventoryYAML: `packages:
  - nginx
  - php8.2
  - php8.2-fpm
  - php8.2-cli
  - php8.2-mysql
  - php8.2-mbstring
  - php8.2-xml
  - php8.2-zip
  - php8.2-curl
  - composer

services:
  - name: nginx
    enabled: true
    state: started
  - name: php8.2-fpm
    enabled: true
    state: started
`,
	},

	"java": {
		label:         "Java",
		suggestedSrc:  "./target",
		suggestedDest: "/opt",
		mappingHint: `  Maven — src: ./target       → /opt/<name>/
  Gradle — src: ./build/libs  → /opt/<name>/
  mode: "0644"
  exclude: "*.java", classes/, generated-sources/

  Runtime: java -jar /opt/<name>/<name>.jar
  Tip: use a systemd unit (.service) to manage the process.`,
		inventoryYAML: `packages:
  - openjdk-21-jre-headless
  # - openjdk-17-jre-headless   # uncomment for Java 17
`,
	},

	"go": {
		label:         "Go",
		suggestedSrc:  "./dist",
		suggestedDest: "/usr/local/bin",
		mappingHint: `  src:  ./dist          → /usr/local/bin/
  mode: "0755"

  Go binaries are statically linked — no runtime packages required.
  Tip: use a systemd unit (.service) to manage the process.`,
		inventoryYAML: `# Go binaries are statically linked — no runtime packages required.
# Add packages below if your app shells out to system tools.
packages: []
`,
	},

	"rust": {
		label:         "Rust",
		suggestedSrc:  "./target/release",
		suggestedDest: "/usr/local/bin",
		mappingHint: `  src:  ./target/release → /usr/local/bin/
  mode: "0755"
  exclude: "*.d", "*.rlib", build/, deps/

  Rust binaries are statically linked — no runtime packages required.
  Tip: use a systemd unit (.service) to manage the process.`,
		inventoryYAML: `# Rust binaries are statically linked — no runtime packages required.
# Add packages below if your app shells out to system tools.
packages: []
`,
	},

	"nginx": {
		label:         "Static site (nginx)",
		suggestedSrc:  "./dist",
		suggestedDest: "/var/www",
		mappingHint: `  src:  ./dist          → /var/www/<name>/
  mode: "0644"

  nginx serves files from /var/www/<name>.
  Tip: deploy your nginx vhost config via a second mapping:
    src:  .simplecd/<name>.conf  → /etc/nginx/sites-enabled/<name>
  Then reload nginx in server_post: systemctl reload nginx`,
		inventoryYAML: `packages:
  - nginx

services:
  - name: nginx
    enabled: true
    state: started
`,
	},
}

// detectedKeyFor maps detectProjectType() output to a stackTemplates key.
func detectedKeyFor(projectType string) string {
	return map[string]string{
		"Node.js":    "nodejs",
		"Python":     "python",
		"PHP/Laravel": "laravel",
		"Go":         "go",
		"Rust":       "rust",
	}[projectType]
}
