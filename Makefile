# Convenience targets. Canonical logic lives in mise.toml + .mise-tasks/.
# Prefer: mise run <task>
# This Makefile runs the same recipes / bash file-tasks directly (no `mise run`).
# Do not name Make targets with colons (mise `kind:up` → Make `kind-up`).
# `.PHONY: kind:up` is a static pattern rule; GNU Make: "target pattern contains no '%'".

IMG ?= zrok-operator:dev
export IMG

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
TASKS := $(ROOT)/.mise-tasks

SHELL := /usr/bin/env bash
.SHELLFLAGS := -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: ## Generate CRDs + RBAC
	controller-gen rbac:roleName=manager-role crd paths=./... output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: ## Generate DeepCopy methods
	controller-gen object:headerFile=hack/boilerplate.go.txt paths=./...

.PHONY: gen-mocks
gen-mocks: ## Generate mocks via mockery
	$(TASKS)/gen/mocks

.PHONY: gen
gen: generate manifests gen-mocks ## generate + manifests + mocks + sync Helm CRDs
	cp config/crd/bases/*.yaml charts/zrok-operator/crds/

.PHONY: fmt
fmt: ## go fmt
	go fmt ./...

.PHONY: vet
vet: ## go vet
	go vet ./...

.PHONY: test
test: gen fmt vet ## Unit + envtest (excludes e2e)
	@ROOT="$(ROOT)" BIN_DIR="$(ROOT)/bin" bash -ec '\
	  ASSETS="$$(setup-envtest use 1.36 --bin-dir "$$BIN_DIR" -p path)"; \
	  KUBEBUILDER_ASSETS="$$ASSETS" gotestsum --format testdox -- \
	    $$(go list ./... | grep -v /e2e) -coverprofile cover.out'

.PHONY: test-verbose
test-verbose: gen ## Verbose unit + envtest
	@ROOT="$(ROOT)" BIN_DIR="$(ROOT)/bin" bash -ec '\
	  ASSETS="$$(setup-envtest use 1.36 --bin-dir "$$BIN_DIR" -p path)"; \
	  KUBEBUILDER_ASSETS="$$ASSETS" gotestsum --format standard-verbose -- \
	    $$(go list ./... | grep -v /e2e) -coverprofile cover.out'

.PHONY: test-e2e
test-e2e: ## Kind e2e (set ZROK2_ENABLE_TOKEN for live share)
	IMG="$(IMG)" $(TASKS)/test-e2e

.PHONY: lint
lint: ## golangci-lint
	golangci-lint run

.PHONY: lint-fix
lint-fix: ## golangci-lint --fix
	golangci-lint run --fix

##@ Build

.PHONY: build
build: gen fmt vet ## Build manager binary to bin/manager
	$(TASKS)/build

.PHONY: run
run: gen ## Run manager against current kubecontext
	@source "$(TASKS)/lib/version.sh" && go run -trimpath -buildvcs=true -ldflags "$$GOLDFLAGS" ./cmd

.PHONY: docker-build
docker-build: ## Build manager Docker image
	IMG="$(IMG)" $(TASKS)/docker-build

.PHONY: docker-push
docker-push: ## Push manager Docker image
	docker push "$(IMG)"

##@ Deployment

.PHONY: install
install: manifests ## Install CRDs into current cluster
	kubectl apply -f config/crd/bases

.PHONY: uninstall
uninstall: ## Remove CRDs from current cluster
	kubectl delete -f config/crd/bases --ignore-not-found=true

.PHONY: deploy
deploy: ## Deploy operator via kustomize
	IMG="$(IMG)" $(TASKS)/deploy

.PHONY: undeploy
undeploy: ## Undeploy operator
	$(TASKS)/undeploy

.PHONY: kind-up
kind-up: ## Create Kind cluster (mise: kind:up)
	$(TASKS)/kind/up

.PHONY: kind-load
kind-load: ## Load image into Kind (mise: kind:load)
	IMG="$(IMG)" $(TASKS)/kind/load

.PHONY: kind-deploy
kind-deploy: ## Build, load, and deploy to Kind (mise: kind:deploy)
	IMG="$(IMG)" $(TASKS)/kind/deploy

.PHONY: samples
samples: ## Apply sample CRs (SECRET=1 to create zrok-credentials from env)
	usage_secret="$(if $(filter 1 true,$(SECRET)),true,false)" $(TASKS)/samples

.PHONY: validate
validate: manifests ## Validate CRDs with kubeconform
	$(TASKS)/validate

.PHONY: helm-package
helm-package: gen ## Package Helm chart
	$(TASKS)/helm-package
