
BASE_IMAGE = amd64/golang:1.17-alpine3.12
LINT_IMAGE = golangci/golangci-lint:v1.38.0

.PHONY: $(shell ls)

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy                      run go mod tidy"
	@echo "  format                        format source files"
	@echo "  test                          run tests"
	@echo "  lint                          run linter"
	@echo "  test-manual                   start a test hub and client"
	@echo "  run-example E=[name]          run an example by name"
	@echo "  run-command N=[name] A=[args] run a command by name"
	@echo ""

blank :=
define NL

$(blank)
endef

mod-tidy:
	docker run --rm -it -v $(PWD):/s -w /s $(BASE_IMAGE) \
	sh -c "apk add git && GOPROXY=direct go get && go mod tidy"

define DOCKERFILE_FORMAT
FROM $(BASE_IMAGE)
RUN apk add --no-cache git
RUN GO111MODULE=on go get mvdan.cc/gofumpt
endef
export DOCKERFILE_FORMAT

format:
	echo "$$DOCKERFILE_FORMAT" | docker build -q . -f - -t temp
	docker run --rm -it -v $(PWD):/s -w /s temp \
	sh -c "find . -type f -name '*.go' | xargs gofumpt -l -w"

define DOCKERFILE_TEST
FROM $(BASE_IMAGE)
RUN apk add --no-cache make docker-cli gcc musl-dev
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
endef
export DOCKERFILE_TEST

test-cmd:
	go build -o /dev/null ./cmd/...

test-examples:
	go build -o /dev/null ./examples/...

test-root:
	$(foreach HUB,$(shell echo testimages/*/ | xargs -n1 basename), \
	docker build -q testimages/$(HUB) -t dctk-test-hub-$(HUB)$(NL))
	go test -v -race -coverprofile=coverage-test.txt .

test-nodocker: test-cmd test-examples test-root

test:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t dctk-test
	docker run --rm -it \
	--name dctk-test \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	dctk-test \
	make test-nodocker

lint:
	docker run --rm -v $(PWD):/app -w /app \
	$(LINT_IMAGE) \
	golangci-lint run -v

test-manual:
	cd ./test-manual && docker-compose up \
	--build \
	--renew-anon-volumes \
	--force-recreate

run-example:
	@test -f "./examples/$(E).go" || ( echo "example file not found"; exit 1 )
	docker run --rm -it -v $(PWD):/s -w /s \
	--network=host \
	$(BASE_IMAGE) \
	sh -c "go run examples/$(E).go"

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
