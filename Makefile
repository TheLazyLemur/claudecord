run:
	go run ./cmd/claudecord

test:
	go test -v ./...

IMAGE_NAME := claudecord
CONTAINER_NAME := claudecord

podman-build:
	podman build -t $(IMAGE_NAME) .

GH_TOKEN := $(shell gh auth token)

podman-run: podman-build
	podman run -d --name $(CONTAINER_NAME) \
		-p 8080:8080 \
		-v ~/.claude:/root/.claude \
		-v ~/.ssh:/root/.ssh:ro \
		-v ~/.gitconfig:/root/.gitconfig:ro \
		-e GH_TOKEN=$(GH_TOKEN) \
		--env-file .env \
		$(IMAGE_NAME)

podman-down:
	podman stop $(CONTAINER_NAME) || true
	podman rm $(CONTAINER_NAME) || true
