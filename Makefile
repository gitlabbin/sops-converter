
# Image URL to use all building/pushing image targets
IMG ?= docker.io/$(DOCKER_USER)/sops-converter:v0.0.9
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: fmt vet generate manifests mocks

build: generate manifests fmt vet
	go build -o bin/sops-converter github.com/dhouti/sops-converter/cli
	go build -o bin/controller github.com/dhouti/sops-converter

release-cli: build-cli zip-cli

build-cli: generate manifests fmt vet
	GOOS=darwin GOARCH=amd64 go build -o bin/sops-converter-cli-darwin-amd64 github.com/dhouti/sops-converter/cli
	GOOS=linux GOARCH=amd64 go build -o bin/sops-converter-cli-linux-amd64 github.com/dhouti/sops-converter/cli
	GOOS=linux GOARCH=arm64 go build -o bin/sops-converter-cli-linux-arm64 github.com/dhouti/sops-converter/cli
	GOOS=windows GOARCH=amd64 go build -o bin/sops-converter-cli-windows-amd64 github.com/dhouti/sops-converter/cli

zip-cli:
	zip -rq bin/sops-converter-cli-windows-amd64.zip bin/sops-converter-cli-windows-amd64
	tar czf bin/sops-converter-cli-darwin-amd64.tgz bin/sops-converter-cli-darwin-amd64
	tar czf bin/sops-converter-cli-linux-amd64.tgz bin/sops-converter-cli-linux-amd64
	tar czf bin/sops-converter-cli-linux-arm64.tgz bin/sops-converter-cli-linux-arm64

# Run tests
test: generate mocks manifests fmt vet
	go test ./... -coverprofile cover.out

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..."

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

mocks:
	go install github.com/matryer/moq@latest
	go generate controllers/sopssecret_controller.go

# Build the docker image
docker: docker-build docker-push

docker-build:
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker login -u $(DOCKER_USER) -p $(DOCKER_PWD)
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.7.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
