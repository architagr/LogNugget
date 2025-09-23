.PHONY: fmt vet test test-coverage
outDir := .out
coverageFile := $(outDir)/coverage.txt
coverageHTML := $(outDir)/coverage.html

fmt:
	gofmt -w .

test:
	go test ./... -race -v -covermode=atomic

vet:
	go vet ./...

test-coverage:
	mkdir -p .out
	go test ./... -coverprofile $(coverageFile) -covermode=atomic 
	go tool cover -html=$(coverageFile) -o $(coverageHTML)
	@echo "Coverage report generated at $(coverageHTML)"
	@echo "Opening coverage report in browser..."
	open -a "Safari" $(coverageHTML)

pprof-report:
	go tool pprof -http=":8086" http://localhost:6060/debug/pprof/profile