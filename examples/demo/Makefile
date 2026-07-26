.DEFAULT_GOAL := help

# Local demo: a real Uptime Kuma in Docker, driven by the provider built from
# this repository. Nothing is published or installed — Terraform is pointed at
# the local binary through dev_overrides.

SHELL         := /bin/bash
PROVIDER_DIR  := $(abspath ../..)
BINARY        := $(PROVIDER_DIR)/terraform-provider-uptimekuma

KUMA_PORT     ?= 3001
KUMA_VERSION  ?= 2.4.0
KUMA_URL      ?= http://localhost:$(KUMA_PORT)
KUMA_USERNAME ?= demo
KUMA_PASSWORD ?= demo123

# dev_overrides makes Terraform use the freshly built binary instead of the
# registry. It also means `terraform init` is neither needed nor useful here.
TERRAFORMRC   := $(CURDIR)/terraformrc
TF            := TF_CLI_CONFIG_FILE=$(TERRAFORMRC) terraform
TF_VARS       := -var endpoint=$(KUMA_URL) -var username=$(KUMA_USERNAME) -var password=$(KUMA_PASSWORD)

export KUMA_PORT
export KUMA_VERSION

## ── Help ────────────────────────────────────────────────────────────
.PHONY: help
help:
	@echo "Uptime Kuma provider demo"
	@echo
	@echo "  make demo       start the container and apply everything (start here)"
	@echo "  make verify     apply, prove the plan is then empty, and destroy (what CI runs)"
	@echo "  make up         start Uptime Kuma and create the admin user"
	@echo "  make apply      build the provider and apply main.tf"
	@echo "  make plan       show what would change (empty plan means no drift)"
	@echo "  make show       print the outputs again"
	@echo "  make destroy    remove everything Terraform created, container stays up"
	@echo "  make down       stop the container and delete its data"
	@echo "  make logs       follow the container logs"
	@echo "  make open       print the URL and credentials"
	@echo
	@echo "  URL:      $(KUMA_URL)"
	@echo "  user:     $(KUMA_USERNAME)"
	@echo "  password: $(KUMA_PASSWORD)"

## ── Container ───────────────────────────────────────────────────────
.PHONY: up
up: wait bootstrap open

.PHONY: start
start:
	docker compose up -d

.PHONY: wait
wait: start
	@echo "waiting for Uptime Kuma to finish its migrations..."
	@for i in $$(seq 1 60); do \
		if curl -fsS "$(KUMA_URL)/socket.io/?EIO=4&transport=polling" >/dev/null 2>&1; then \
			echo "ready"; exit 0; \
		fi; \
		sleep 2; \
	done; \
	echo "gave up waiting; check 'make logs'"; exit 1

# A fresh instance has no account, and only the setup event can create one.
# Running this twice is harmless.
.PHONY: bootstrap
bootstrap:
	@cd $(PROVIDER_DIR) && go run ./tools/kuma-bootstrap \
		-endpoint $(KUMA_URL) -username $(KUMA_USERNAME) -password '$(KUMA_PASSWORD)'

.PHONY: down
down:
	docker compose down -v
	@rm -f terraform.tfstate terraform.tfstate.backup $(TERRAFORMRC)

.PHONY: logs
logs:
	docker compose logs -f uptime-kuma

.PHONY: open
open:
	@echo
	@echo "Uptime Kuma is up:  $(KUMA_URL)"
	@echo "  user:     $(KUMA_USERNAME)"
	@echo "  password: $(KUMA_PASSWORD)"
	@echo

## ── Terraform ───────────────────────────────────────────────────────
.PHONY: build
build:
	@cd $(PROVIDER_DIR) && go build -o $(BINARY) .
	@echo "built $(BINARY)"

$(TERRAFORMRC):
	@printf 'provider_installation {\n  dev_overrides {\n    "DiegoBulhoes/uptimekuma" = "%s"\n  }\n  direct {}\n}\n' "$(PROVIDER_DIR)" > $@
	@echo "wrote $@"

# init registers the local modules under ./modules. It is needed even with
# dev_overrides, which only covers the provider.
.PHONY: init
init: $(TERRAFORMRC)
	$(TF) init -input=false

.PHONY: apply
apply: build init
	$(TF) apply -auto-approve $(TF_VARS)

.PHONY: plan
plan: build init
	$(TF) plan $(TF_VARS)

.PHONY: show
show: $(TERRAFORMRC)
	$(TF) output

.PHONY: destroy
destroy: build init
	$(TF) destroy -auto-approve $(TF_VARS)

# verify is what CI runs: apply, prove a second plan is empty, then tear down.
# A non-empty plan after apply means the provider is not reporting some value
# back the way it wrote it.
.PHONY: verify
verify: apply
	@echo
	@echo "checking the configuration is idempotent..."
	@$(TF) plan -detailed-exitcode $(TF_VARS) > /dev/null; \
	code=$$?; \
	if [ $$code -eq 0 ]; then \
		echo "✓ second plan is empty"; \
	elif [ $$code -eq 2 ]; then \
		echo "✗ second plan wants changes:"; \
		$(TF) plan $(TF_VARS); \
		exit 1; \
	else \
		echo "✗ plan failed"; \
		exit $$code; \
	fi

.PHONY: fmt
fmt:
	terraform fmt

## ── All in one ──────────────────────────────────────────────────────
.PHONY: demo
demo: up apply
	@echo
	@echo "Everything applied. Open $(KUMA_URL) and log in as $(KUMA_USERNAME) / $(KUMA_PASSWORD)."
	@echo "The container stays up until you run 'make down'."
