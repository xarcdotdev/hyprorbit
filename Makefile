.PHONY: build tidy lint test clean

build:
	go build -o hyprorbit ./cmd/hyprorbit
	go build -o hyprorbitd ./cmd/hyprorbitd

tidy:
	go mod tidy

lint:
	go vet ./...

test:
	go test ./...

clean:
	rm -f hyprorbit hyprorbitd
