.PHONY: build tidy lint clean

build:
	go build -o hyprorbits ./cmd/hyprorbits
	go build -o hyprorbitsd ./cmd/hyprorbitsd

tidy:
	go mod tidy

lint:
	go vet ./...

clean:
	rm -f hyprorbits hyprorbitsd
