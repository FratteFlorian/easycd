.PHONY: build build-client build-server install-server test clean

build-client:
	go build -o dist/simplecd ./cmd/simplecd

build-server:
	GOOS=linux GOARCH=amd64 go build -o dist/simplecdd ./cmd/simplecdd

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
	ssh root@$(CT_HOST) "mkdir -p /etc/simplecd /var/log/simplecd /var/lib/simplecd/.global"
	scp dist/simplecdd root@$(CT_HOST):/usr/local/bin/simplecdd
	scp install/simplecdd.service root@$(CT_HOST):/etc/systemd/system/simplecdd.service
	ssh root@$(CT_HOST) "systemctl daemon-reload && systemctl enable --now simplecdd"
	@echo ""
	@echo "Bootstrap complete!"
	@echo "Now create /etc/simplecd/server.yaml on the CT:"
	@echo ""
	@echo "  ssh root@$(CT_HOST)"
	@echo "  cat > /etc/simplecd/server.yaml << 'EOF'"
	@echo "  listen: :8765"
	@echo "  token: $$(openssl rand -hex 32)"
	@echo "  log_dir: /var/log/simplecd"
	@echo "  EOF"
	@echo "  systemctl restart simplecdd"
