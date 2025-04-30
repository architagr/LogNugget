.PHONY: fmt vet test test-coverage

fmt:
	gofmt -w .

test:
	go test ./...

vet:
	go vet ./...

test-coverage:
	go test ./... -coverprofile coverage.txt -covermode=atomic 
	go tool cover -html=coverage.txt -o coverage.html