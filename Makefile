IMG ?= ghcr.io/ankitavasudev/quota-guard:latest
CONTROLLER_GEN ?= $(shell which controller-gen)

.PHONY: help test vet lint run build docker-build deploy undeploy demo

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

test: ## Run unit tests
	go test ./... -count=1

vet: ## Run go vet
	go vet ./...

build: ## Build the manager binary
	CGO_ENABLED=0 go build -o bin/quota-guard ./cmd/quota-guard

run: ## Run the controller against your current kubeconfig (needs a cluster)
	go run ./cmd/quota-guard

docker-build: ## Build the container image
	docker build -t $(IMG) .

deploy: ## Apply CRD, RBAC and the controller deployment
	kubectl apply -f config/crd/resourcebudgets.guard.ankaita.dev.yaml
	kubectl apply -f config/rbac/rbac.yaml
	kubectl apply -f deploy/deployment.yaml

undeploy: ## Remove the controller
	kubectl delete -f deploy/deployment.yaml --ignore-not-found
	kubectl delete -f config/rbac/rbac.yaml --ignore-not-found

demo-apply: ## Create a sample namespace + budget + pods (kind cluster)
	kubectl create namespace demo --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f config/samples/resourcebudget.yaml

demo-watch: ## Watch the budget status update (kubectl get resourcebudgets.guard.ankaita.dev -n demo -w)
	kubectl get resourcebudgets.guard.ankaita.dev -n demo -w

clean: ## Remove build artifacts
	rm -rf bin dist
