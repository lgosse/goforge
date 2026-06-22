PACKAGE     = goforge
DATE       ?= $(shell date +%FT%T%z)
VERSION    ?= $(shell git describe --tags --always)

PKG_LIST    = $(shell go list ./... | grep -v /vendor/ | grep -v /scripts/)

GO          = go
GOLINT      = ./bin/custom-gcl
GOLINT_SRC ?= golangci-lint
GODOC       = godoc
GOFMT       = gofmt

V           = 0
Q           = $(if $(filter 1,$V),,@)
M           = $(shell printf "\033[0;35m▶\033[0m")


.PHONY: all
all: vendor check

# Vendor
.PHONY: vendor
vendor: ## Create vendor directory from go.sum
	$(info $(M) running mod vendor…) @
	$Q $(GO) mod vendor

# Tidy
.PHONY: tidy
tidy: ## Update go.sum with go.mod
	$(info $(M) running mod tidy…) @
	$Q $(GO) mod tidy

# Lint
.PHONY: custom-gcl
custom-gcl: .custom-gcl.yml ## Build custom golangci-lint with local plugins
	$(info $(M) building custom golangci-lint…)
	$Q mkdir -p ./bin
	$Q proxy="$${GOPROXY:-https://proxy.golang.org,direct}"; \
		case "$$proxy" in \
			off|*proxy.golang.org*|*direct*) ;; \
			*) proxy="$$proxy,https://proxy.golang.org,direct" ;; \
		esac; \
		env GOFLAGS="$${GOFLAGS:+$${GOFLAGS} }-mod=mod" GOPROXY="$$proxy" GONOSUMDB="$${GONOSUMDB:-gitlab.side.co/*}" $(GOLINT_SRC) custom --name custom-gcl --destination ./bin

.PHONY: lint
lint: custom-gcl ## Run linter check on project
	$(info $(M) running $(GOLINT)…)
	$Q $(GOLINT) run

.PHONY: lint-fix
lint-fix: custom-gcl ## Run linter autofixes on project
	$(info $(M) running $(GOLINT) --fix…)
	$Q $(GOLINT) run --fix

# Test
.PHONY: test
test: ## Run unit tests
	$(info $(M) running go test…) @
	$Q $(GO) test -cover -race -v ./...

# Check
.PHONY: check
check: vendor lint test

.PHONY: doc
doc: ## Run godoc on project
	$(info $(M) running $(GODOC)…) @
	$Q $(GODOC) ./...

.PHONY: clean
clean: ## Clean previously built binaries
	$(info $(M) cleaning…)	@ ## Cleanup everything
	@rm -rf bin/$(PACKAGE)_*

.PHONY: help
help:
	@grep -E '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: version
version: ## Print current project version
	@echo $(VERSION)