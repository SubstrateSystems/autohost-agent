.PHONY: build clean install uninstall run test release deploy-vm vm-start vm-stop vm-status vm-logs vm-shell deploy-incus incus-start incus-stop incus-status incus-logs incus-shell

BINARY_NAME=autohost-agent
INSTALL_PATH=/usr/local/bin
CONFIG_PATH=/etc/autohost
SERVICE_PATH=/etc/systemd/system
VM_NAME=autohost-test
INCUS_INSTANCE=autohost-test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  = -s -w -X main.Version=$(VERSION)
PLATFORMS = linux/amd64 linux/arm64

build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) cmd/agent/main.go
	@echo "Build complete: ./$(BINARY_NAME)"

release:
	@echo "🚀 Building release $(VERSION) for: $(PLATFORMS)"
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/}; \
		out="dist/$(BINARY_NAME)-$${GOOS}-$${GOARCH}"; \
		echo "  → $${out}"; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "$(LDFLAGS)" -o "$$out" cmd/agent/main.go; \
	done
	@echo "🔐 Generating checksums..."
	@cd dist && sha256sum $(BINARY_NAME)-* > checksums_$(VERSION).txt
	@echo "✅ Release artifacts in dist/"
	@ls -lh dist/

clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	@echo "Clean complete"

install: build
	@echo "Installing $(BINARY_NAME)..."
	sudo mkdir -p $(CONFIG_PATH)
	sudo cp $(BINARY_NAME) $(INSTALL_PATH)/
	sudo cp autohost-agent.service $(SERVICE_PATH)/
	@if [ ! -f $(CONFIG_PATH)/config.yaml ]; then \
		sudo cp configs/agent.yaml $(CONFIG_PATH)/config.yaml; \
		sudo chmod 600 $(CONFIG_PATH)/config.yaml; \
		echo "Created config file at $(CONFIG_PATH)/config.yaml - PLEASE EDIT IT"; \
	fi
	sudo systemctl daemon-reload
	@echo "Installation complete. Edit $(CONFIG_PATH)/config.yaml and run 'make enable' to start the service"

uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	sudo systemctl stop $(BINARY_NAME) 2>/dev/null || true
	sudo systemctl disable $(BINARY_NAME) 2>/dev/null || true
	sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	sudo rm -f $(SERVICE_PATH)/$(BINARY_NAME).service
	sudo systemctl daemon-reload
	@echo "Uninstall complete. Config files in $(CONFIG_PATH) were preserved"

enable:
	sudo systemctl enable $(BINARY_NAME)
	sudo systemctl start $(BINARY_NAME)
	@echo "Service enabled and started"

disable:
	sudo systemctl stop $(BINARY_NAME)
	sudo systemctl disable $(BINARY_NAME)
	@echo "Service stopped and disabled"

status:
	sudo systemctl status $(BINARY_NAME)

logs:
	sudo journalctl -u $(BINARY_NAME) -f

run: build
	./$(BINARY_NAME) config.example.yaml

test:
	go test -v ./...

deploy-vm: build
	@echo "Deploying to VM $(VM_NAME)..."
	@echo "1. Transferring files..."
	multipass transfer $(BINARY_NAME) $(VM_NAME):/home/ubuntu/
	multipass transfer configs/agent.yaml $(VM_NAME):/home/ubuntu/
	multipass transfer autohost-agent.service $(VM_NAME):/home/ubuntu/
	@echo "2. Installing on VM..."
	multipass exec $(VM_NAME) -- sudo mkdir -p $(CONFIG_PATH)
	multipass exec $(VM_NAME) -- sudo cp /home/ubuntu/$(BINARY_NAME) $(INSTALL_PATH)/
	multipass exec $(VM_NAME) -- sudo cp /home/ubuntu/agent.yaml $(CONFIG_PATH)/config.yaml
	multipass exec $(VM_NAME) -- sudo chmod 600 $(CONFIG_PATH)/config.yaml
	multipass exec $(VM_NAME) -- sudo cp /home/ubuntu/autohost-agent.service $(SERVICE_PATH)/
	multipass exec $(VM_NAME) -- sudo systemctl daemon-reload
	@echo "3. Cleaning up temporary files..."
	multipass exec $(VM_NAME) -- rm /home/ubuntu/$(BINARY_NAME) /home/ubuntu/agent.yaml /home/ubuntu/autohost-agent.service
	@echo ""
	@echo "✓ Deployment complete!"
	@echo "  Binary installed at: $(INSTALL_PATH)/$(BINARY_NAME)"
	@echo "  Config file at: $(CONFIG_PATH)/config.yaml"
	@echo "  Service file at: $(SERVICE_PATH)/autohost-agent.service"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Edit config: multipass exec $(VM_NAME) -- sudo nano $(CONFIG_PATH)/config.yaml"
	@echo "  2. Enable service: multipass exec $(VM_NAME) -- sudo systemctl enable autohost-agent"
	@echo "  3. Start service: multipass exec $(VM_NAME) -- sudo systemctl start autohost-agent"
	@echo "  4. Check status: multipass exec $(VM_NAME) -- sudo systemctl status autohost-agent"

vm-start:
	@echo "Starting service on VM..."
	multipass exec $(VM_NAME) -- sudo systemctl enable autohost-agent
	multipass exec $(VM_NAME) -- sudo systemctl start autohost-agent
	@echo "Service started. Use 'make vm-status' to check status"

vm-stop:
	@echo "Stopping service on VM..."
	multipass exec $(VM_NAME) -- sudo systemctl stop autohost-agent
	@echo "Service stopped"

vm-status:
	multipass exec $(VM_NAME) -- sudo systemctl status autohost-agent

vm-logs:
	multipass exec $(VM_NAME) -- sudo journalctl -u autohost-agent -f

vm-shell:
	multipass shell $(VM_NAME)

# Incus deployment and management targets
deploy-incus: build
	@echo "Deploying to Incus instance $(INCUS_INSTANCE)..."

	@echo "1. Transferring files..."
	incus file push $(BINARY_NAME) $(INCUS_INSTANCE)/home/ubuntu/
	incus file push configs/agent.yaml $(INCUS_INSTANCE)/home/ubuntu/
	incus file push autohost-agent.service $(INCUS_INSTANCE)/home/ubuntu/

	@echo "2. Installing on instance..."

	# Create system user if not exists
	incus exec $(INCUS_INSTANCE) -- sudo id -u autohost >/dev/null 2>&1 || \
		incus exec $(INCUS_INSTANCE) -- sudo useradd --system --no-create-home --shell /usr/sbin/nologin autohost

	# Create config directory
	incus exec $(INCUS_INSTANCE) -- sudo mkdir -p /etc/autohost

	# Install binary
	incus exec $(INCUS_INSTANCE) -- sudo mv /home/ubuntu/$(BINARY_NAME) /usr/local/bin/
	incus exec $(INCUS_INSTANCE) -- sudo chown root:root /usr/local/bin/$(BINARY_NAME)
	incus exec $(INCUS_INSTANCE) -- sudo chmod 755 /usr/local/bin/$(BINARY_NAME)

	# Install config
	incus exec $(INCUS_INSTANCE) -- sudo mv /home/ubuntu/agent.yaml /etc/autohost/config.yaml
	incus exec $(INCUS_INSTANCE) -- sudo chown root:autohost /etc/autohost/config.yaml
	incus exec $(INCUS_INSTANCE) -- sudo chmod 640 /etc/autohost/config.yaml

	# Install service
	incus exec $(INCUS_INSTANCE) -- sudo mv /home/ubuntu/autohost-agent.service /etc/systemd/system/
	incus exec $(INCUS_INSTANCE) -- sudo systemctl daemon-reload

	@echo "3. Cleaning temporary files..."
	incus exec $(INCUS_INSTANCE) -- rm -f /home/ubuntu/$(BINARY_NAME) /home/ubuntu/agent.yaml /home/ubuntu/autohost-agent.service

	@echo ""
	@echo "✓ Deployment complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Enable service: incus exec $(INCUS_INSTANCE) -- sudo systemctl enable autohost-agent"
	@echo "  2. Start service:  incus exec $(INCUS_INSTANCE) -- sudo systemctl start autohost-agent"
	@echo "  3. Check status:   incus exec $(INCUS_INSTANCE) -- sudo systemctl status autohost-agent"

deploy-incus-update: build
	@echo "Updating Incus instance $(INCUS_INSTANCE)..."
	@echo "1. Transferring new binary..."
	incus file push $(BINARY_NAME) $(INCUS_INSTANCE)/home/ubuntu/
	@echo "2. Updating binary on instance..."
	incus exec $(INCUS_INSTANCE) -- sudo mv /home/ubuntu/$(BINARY_NAME) /usr/local/bin/
	incus exec $(INCUS_INSTANCE) -- sudo chown root:root /usr/local/bin/$(BINARY_NAME)
	incus exec $(INCUS_INSTANCE) -- sudo chmod 755 /usr/local/bin/$(BINARY_NAME)
	@echo "3. Cleaning temporary files..."
	incus exec $(INCUS_INSTANCE) -- rm -f /home/ubuntu/$(BINARY_NAME)
	@echo "Update complete. Restart the service to apply changes."

incus-start:
	@echo "Starting service on Incus instance..."
	incus exec $(INCUS_INSTANCE) -- sudo systemctl enable autohost-agent
	incus exec $(INCUS_INSTANCE) -- sudo systemctl start autohost-agent
	@echo "Service started. Use 'make incus-status' to check status"

incus-stop:
	@echo "Stopping service on Incus instance..."
	incus exec $(INCUS_INSTANCE) -- sudo systemctl stop autohost-agent
	@echo "Service stopped"

incus-status:
	incus exec $(INCUS_INSTANCE) -- sudo systemctl status autohost-agent

incus-logs:
	incus exec $(INCUS_INSTANCE) -- sudo journalctl -u autohost-agent -f

incus-shell:
	incus exec $(INCUS_INSTANCE) -- bash
