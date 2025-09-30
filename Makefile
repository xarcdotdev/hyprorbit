.PHONY: build tidy lint clean

build:
	go build -o hyprorbit ./cmd/hyprorbit
	go build -o hyprorbitd ./cmd/hyprorbitd

tidy:
	go mod tidy

lint:
	go vet ./...

clean:
	rm -f hyprorbit hyprorbitd
