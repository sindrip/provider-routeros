.PHONY: generate sync-package-crds test envtest build image package package-all verify-package

CONTROLLER_GEN_VERSION ?= v0.20.1
SETUP_ENVTEST_VERSION ?= v0.24.1
ENVTEST_KUBERNETES_VERSION ?= 1.36.2

ARCH ?= $(shell go env GOARCH)
ARCHES ?= amd64 arm64
VERSION ?= dev
OUTPUT_DIR ?= ../.work/native
CONTROLLER_IMAGE ?= provider-routeros-native-controller:$(VERSION)-$(ARCH)
PACKAGE_FILE ?= $(OUTPUT_DIR)/provider-routeros-native-$(VERSION)-$(ARCH).xpkg
DOCKER_ENDPOINT ?= $(shell docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null)

PACKAGE_CRDS := \
	ip.routeros.m.sindrip.io_firewallfiltermenus.yaml \
	routeros.m.sindrip.io_providerconfigs.yaml \
	routeros.m.sindrip.io_providerconfigusages.yaml

generate:
	go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION) \
		object paths=./api/... \
		crd paths=./api/... output:crd:artifacts:config=config/crd
	$(MAKE) sync-package-crds

sync-package-crds:
	mkdir -p package/crds
	set -e; for crd in $(PACKAGE_CRDS); do \
		install -m 0644 "config/crd/$$crd" "package/crds/$$crd"; \
	done

test:
	go test ./...

envtest:
	KUBEBUILDER_ASSETS="$$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION) use -p path $(ENVTEST_KUBERNETES_VERSION))" \
		go test ./internal/integration -v

build:
	mkdir -p "$(OUTPUT_DIR)/image/bin/linux_$(ARCH)"
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -trimpath \
		-ldflags='-s -w' \
		-o "$(OUTPUT_DIR)/image/bin/linux_$(ARCH)/provider" ./cmd/provider

image: build
	docker build \
		--platform "linux/$(ARCH)" \
		--build-arg TARGETOS=linux \
		--build-arg TARGETARCH="$(ARCH)" \
		--tag "$(CONTROLLER_IMAGE)" \
		--file cluster/images/provider-routeros/Dockerfile \
		"$(OUTPUT_DIR)/image"

verify-package: sync-package-crds
	@test "$$(find package/crds -type f -name '*.yaml' | wc -l | tr -d ' ')" = "3"
	@! grep -R -n -E 'kind: ClusterProviderConfig|scope: Cluster' package
	@set -e; for crd in $(PACKAGE_CRDS); do \
		cmp -s "config/crd/$$crd" "package/crds/$$crd"; \
	done

package: verify-package image
	mkdir -p "$(OUTPUT_DIR)"
	DOCKER_HOST="$(DOCKER_ENDPOINT)" crossplane xpkg build \
		--package-root package \
		--examples-root config/samples \
		--embed-runtime-image "$(CONTROLLER_IMAGE)" \
		--package-file "$(PACKAGE_FILE)"

package-all:
	set -e; for arch in $(ARCHES); do \
		$(MAKE) package ARCH="$$arch"; \
	done
