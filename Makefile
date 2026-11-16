.PHONY: test build docker-build

test:
	go test ./...

build:
	go build ./...

docker-build:
	docker build -t proxmox-host-labeler:dev .
