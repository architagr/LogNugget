.PHONY: fmt vet test test-coverage

fmt:
	gofmt -w .

test:
	go test ./...

vet:
	go vet ./...

test-coverage:
	mkdir -p .out
	go test ./... -coverprofile .out/coverage.txt -covermode=atomic 
	go tool cover -html=.out/coverage.txt -o .out/coverage.html