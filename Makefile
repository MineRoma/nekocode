BINARY := bin/neko

.PHONY: build test check clean

build:
	mkdir -p bin
	go build -trimpath -o $(BINARY) .

test:
	go test ./...

check:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
	go vet ./...
	go test ./...

clean:
	rm -rf bin
