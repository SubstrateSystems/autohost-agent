.PHONY: build clean install uninstall run test release deploy-vm vm-start vm-stop vm-status vm-logs vm-shell incus-setup incus-create deploy-incus deploy-incus-update incus-start incus-stop incus-status incus-logs incus-shell

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
	@CURRENT=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev"); \
	echo "📌 Versión actual: $$CURRENT"; \
	printf "🔖 Nueva versión (ej. v1.2.3): "; \
	read NEW_VERSION; \
	if [ -z "$$NEW_VERSION" ]; then echo "❌ La versión no puede estar vacía"; exit 1; fi; \
	echo "🏷️  Creando tag $$NEW_VERSION..."; \
	git tag -a "$$NEW_VERSION" -m "Release $$NEW_VERSION"; \
	echo "🚀 Compilando release $$NEW_VERSION para: $(PLATFORMS)"; \
	mkdir -p dist; \
	for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; GOARCH=$${platform#*/}; \
		out="dist/$(BINARY_NAME)-$${GOOS}-$${GOARCH}"; \
		echo "  → $$out"; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "-s -w -X main.Version=$$NEW_VERSION" -o "$$out" cmd/agent/main.go; \
	done; \
	echo "🔐 Generando checksums..."; \
	cd dist && sha256sum $(BINARY_NAME)-* > "checksums_$${NEW_VERSION}.txt"; \
	echo "✅ Artefactos en dist/"; \
	ls -lh dist/; \
	echo ""; \
	echo "💡 Para publicar el tag ejecuta: git push origin $$NEW_VERSION"



# Incus setup — instala y configura Incus en esta máquina
incus-setup:
	@echo "📦 Instalando Incus..."
	@if command -v incus >/dev/null 2>&1; then \
		echo "✅ Incus ya está instalado: $$(incus --version)"; \
	else \
		sudo apt-get update -qq && sudo apt-get install -y incus; \
	fi
	@echo "⚙️  Inicializando Incus (modo minimal)..."
	@if ! incus info >/dev/null 2>&1; then \
		echo "{}" | sudo incus admin init --preseed; \
	else \
		echo "✅ Incus ya está inicializado"; \
	fi
	@echo "👤 Añadiendo usuario $$(whoami) al grupo incus..."
	@if ! id -nG $$(whoami) | grep -qw incus; then \
		sudo usermod -aG incus $$(whoami); \
		echo "✅ Usuario añadido al grupo incus"; \
		echo "⚠️  Cierra sesión y vuelve a entrar (o ejecuta: newgrp incus) para aplicar el grupo"; \
	else \
		echo "✅ Ya perteneces al grupo incus"; \
	fi
	@echo ""
	@echo "✓ Incus listo. Crea la instancia de prueba con: make incus-create"

incus-create:
	@echo "🚀 Creando instancia $(INCUS_INSTANCE)..."
	@if incus info $(INCUS_INSTANCE) >/dev/null 2>&1; then \
		echo "✅ La instancia $(INCUS_INSTANCE) ya existe"; \
	else \
		incus launch images:ubuntu/24.04 $(INCUS_INSTANCE); \
		echo "✅ Instancia $(INCUS_INSTANCE) creada"; \
	fi

# Incus deployment and management targets
deploy-incus: build incus-create
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
	@echo "4. Restarting service..."
	incus exec $(INCUS_INSTANCE) -- sudo systemctl restart autohost-agent
	@echo "✓ Update complete and service restarted."

start-incus:
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

shell-incus:
	incus exec $(INCUS_INSTANCE) -- bash
