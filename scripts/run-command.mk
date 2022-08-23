define DOCKERFILE_RUN_COMMAND
FROM $(BASE_IMAGE)
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go install ./...
endef
export DOCKERFILE_RUN_COMMAND

run-command:
	echo "$$DOCKERFILE_RUN_COMMAND" | docker build . -q -f - -t dctk-runcmd
	docker run --rm -it \
	--network=host \
	-e COLUMNS=$(shell tput cols) \
	dctk-runcmd $(N) $(A)
