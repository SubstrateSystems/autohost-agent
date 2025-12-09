.PHONY: build clean install uninstall run test deploy-vm vm-start vm-stop vm-status vm-logs vm-shell

BINARY_NAME=autohost-agent
INSTALL_PATH=/usr/local/bin
CONFIG_PATH=/etc/autohost
SERVICE_PATH=/etc/systemd/system
VM_NAME=autohost-test

build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) cmd/autohost-agent/main.go
	@echo "Build complete: ./$(BINARY_NAME)"

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
		sudo cp config.example.yaml $(CONFIG_PATH)/config.yaml; \
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
	multipass transfer config.example.yaml $(VM_NAME):/home/ubuntu/
	multipass transfer autohost-agent.service $(VM_NAME):/home/ubuntu/
	@echo "2. Installing on VM..."
	multipass exec $(VM_NAME) -- sudo mkdir -p $(CONFIG_PATH)
	multipass exec $(VM_NAME) -- sudo cp /home/ubuntu/$(BINARY_NAME) $(INSTALL_PATH)/
	multipass exec $(VM_NAME) -- sudo cp /home/ubuntu/config.example.yaml $(CONFIG_PATH)/config.yaml
	multipass exec $(VM_NAME) -- sudo chmod 600 $(CONFIG_PATH)/config.yaml
	multipass exec $(VM_NAME) -- sudo cp /home/ubuntu/autohost-agent.service $(SERVICE_PATH)/
	multipass exec $(VM_NAME) -- sudo systemctl daemon-reload
	@echo "3. Cleaning up temporary files..."
	multipass exec $(VM_NAME) -- rm /home/ubuntu/$(BINARY_NAME) /home/ubuntu/config.example.yaml /home/ubuntu/autohost-agent.service
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
