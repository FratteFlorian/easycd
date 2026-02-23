.PHONY: build build-client build-server install-server test clean

build-client:
	go build -o dist/eacd ./cmd/eacd

build-server:
	GOOS=linux GOARCH=amd64 go build -o dist/eacdd ./cmd/eacdd

build: build-client build-server

test:
	go test ./...

clean:
	rm -rf dist/

# One-time bootstrap: copy the server binary to the CT and install it as a service.
# Usage: make install-server CT_HOST=192.168.1.x
install-server:
	@test -n "$(CT_HOST)" || (echo "Usage: make install-server CT_HOST=<host>" && exit 1)
	@echo "Building server binary for Linux amd64..."
	$(MAKE) build-server
	@echo "Copying to $(CT_HOST)..."
	ssh root@$(CT_HOST) "mkdir -p /etc/eacd /var/log/eacd /var/lib/eacd/.global"
	scp dist/eacdd root@$(CT_HOST):/usr/local/bin/eacdd
	scp install/eacdd.service root@$(CT_HOST):/etc/systemd/system/eacdd.service
	ssh root@$(CT_HOST) "systemctl daemon-reload && systemctl enable --now eacdd"
	@echo ""
	@echo "Bootstrap complete!"
	@echo "Now create /etc/eacd/server.yaml on the CT:"
	@echo ""
	@echo "  ssh root@$(CT_HOST)"
	@echo "  cat > /etc/eacd/server.yaml << 'EOF'"
	@echo "  listen: :8765"
	@echo "  token: $$(openssl rand -hex 32)"
	@echo "  log_dir: /var/log/eacd"
	@echo "  EOF"
	@echo "  systemctl restart eacdd"
