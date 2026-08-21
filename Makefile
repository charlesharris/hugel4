.PHONY: build test vet fmt clean

build:
	go build -o bin/hugel ./cmd/hugel

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin
