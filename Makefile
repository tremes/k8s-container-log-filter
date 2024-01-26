.PHONY: build
build: ## Compilation
	go build -o ./k8s-container-log-filter ./cmd/k8s-container-log-filter/main.go

.PHONY: run
run: ## Runs the binary
	./bin/k8s-container-filter