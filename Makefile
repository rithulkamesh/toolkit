# Zero-dependency Go module: every target is plain `go`.
.PHONY: test vet build tidy fmt ci

test:
	go test ./...

vet:
	go vet ./...

build:
	go build ./...

tidy:
	go mod tidy

fmt:
	gofmt -w .

ci: fmt vet test build
