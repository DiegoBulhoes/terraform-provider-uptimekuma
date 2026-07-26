.DEFAULT_GOAL := all

BINARY_NAME   = terraform-provider-uptimekuma
GO            = go
KUMA_VERSIONS ?= 2.2.1 2.3.2 2.4.0

## ── All (CI pipeline) ───────────────────────────────────────────
.PHONY: all
all: tidy lint security test build docs

## ── Build & Install ─────────────────────────────────────────────
.PHONY: build
build:
	$(GO) build -v -o $(BINARY_NAME) .

.PHONY: install
install:
	$(GO) install .

## ── Quality ─────────────────────────────────────────────────────
.PHONY: fmt
fmt:
	$(GO) fmt ./...
	$(GO) tool goimports -w .

.PHONY: lint
lint: fmt
	$(GO) vet ./...
	$(GO) tool golangci-lint run ./...

.PHONY: security
security:
	govulncheck ./...

## ── Tests ───────────────────────────────────────────────────────
.PHONY: test
test:
	$(GO) test -race -count=1 -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/... ./test/unit/...
	$(GO) tool cover -func=coverage.out | tail -1
	@for v in $(KUMA_VERSIONS); do \
		echo "=== Uptime Kuma $$v ==="; \
		UPTIME_KUMA_IMAGE=louislam/uptime-kuma:$$v TF_ACC=1 \
		TF_ACC_TERRAFORM_PATH=$$(which terraform) \
		$(GO) test -p 1 -tags integration -timeout 900s -count=1 ./test/integration/... || exit 1; \
		echo ""; \
	done
	$(GO) tool cover -html=coverage.out -o coverage.html

## ── Docs ────────────────────────────────────────────────────────
# --provider-name is explicit because tfplugindocs otherwise derives it from the
# directory name. Until the repository is renamed to
# terraform-provider-uptimekuma, that guess is "uptime-kuma" and every file looks
# misnamed to the validator.
.PHONY: docs
docs:
	$(GO) tool tfplugindocs generate --provider-name uptimekuma
	$(GO) tool tfplugindocs validate --provider-name uptimekuma

## ── CI parity (matches .github/workflows/test.yml) ──────────────
# `make ci` mirrors the GitHub Actions `checks` + `unit` jobs and does
# NOT modify files (unlike `make lint`, which runs `go fmt` first).
# Run this before pushing to catch CI failures locally.
#
# `make ci-acceptance` adds the acceptance-tests matrix against the
# Uptime Kuma versions in KUMA_VERSIONS (needs Docker + Terraform on PATH).
#
# Acceptance packages run with -p 1: each one starts its own Uptime Kuma
# container, and running three at once starves them enough to cause timeouts.
.PHONY: ci-vet
ci-vet:
	$(GO) vet ./...

.PHONY: ci-fmt-check
ci-fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "ERROR: the following files need formatting (run 'make fmt'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

.PHONY: ci-lint
ci-lint:
	$(GO) tool golangci-lint run ./...

.PHONY: ci-docs
ci-docs:
	$(GO) tool tfplugindocs validate --provider-name uptimekuma

.PHONY: ci-vuln
ci-vuln:
	@command -v govulncheck >/dev/null 2>&1 || { \
		echo "installing govulncheck..."; \
		$(GO) install golang.org/x/vuln/cmd/govulncheck@latest; \
	}
	govulncheck ./...

.PHONY: ci-unit
ci-unit:
	$(GO) test -race -count=1 -coverpkg=./internal/... -coverprofile=coverage-unit.out ./test/unit/...
	@$(GO) tool cover -func=coverage-unit.out | tail -1

.PHONY: ci
ci: ci-vet ci-fmt-check ci-lint ci-docs ci-vuln ci-unit
	@echo
	@echo "✓ CI checks + unit tests pass (mirrors .github/workflows/test.yml 'checks' + 'unit' jobs)"
	@echo "  Acceptance tests require Docker — run 'make ci-acceptance' for full parity."

.PHONY: ci-acceptance
ci-acceptance:
	@for v in $(KUMA_VERSIONS); do \
		echo "=== Uptime Kuma $$v ==="; \
		UPTIME_KUMA_IMAGE=louislam/uptime-kuma:$$v TF_ACC=1 \
		TF_ACC_TERRAFORM_PATH=$$(which terraform) \
		$(GO) test -p 1 -tags integration -timeout 900s -count=1 -coverpkg=./internal/... ./test/integration/... || exit 1; \
		echo ""; \
	done

## ── Housekeeping ────────────────────────────────────────────────
.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY_NAME) coverage.out coverage.html coverage-unit.out
