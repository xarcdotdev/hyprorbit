.PHONY: build tidy lint clean

build:
	go build -o hypr-orbits ./cmd/hypr-orbits
	go build -o hypr-orbitsd ./cmd/hypr-orbitsd

tidy:
	go mod tidy

lint:
	go vet ./...

clean:
	rm -f hypr-orbits hypr-orbitsd
