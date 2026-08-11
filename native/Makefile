.PHONY: generate test envtest

CONTROLLER_GEN_VERSION ?= v0.20.1
SETUP_ENVTEST_VERSION ?= v0.24.1
ENVTEST_KUBERNETES_VERSION ?= 1.36.2

generate:
	go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION) \
		object paths=./api/... \
		crd paths=./api/... output:crd:artifacts:config=config/crd

test:
	go test ./...

envtest:
	KUBEBUILDER_ASSETS="$$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION) use -p path $(ENVTEST_KUBERNETES_VERSION))" \
		go test ./internal/integration -v
