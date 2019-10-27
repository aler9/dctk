
.PHONY: $(shell ls)

BASE_IMAGE = amd64/golang:1.13-alpine3.10

help:
	@echo "usage: make [action] [args...]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy                      run go mod tidy"
	@echo "  format                        format source files"
	@echo "  test                          run available tests"
	@echo "  run-example E=[name]          run an example by name"
	@echo "  run-command N=[name] A=[args] run a command by name"
	@echo ""

blank :=
define NL

$(blank)
endef

mod-tidy:
	docker run --rm -it -v $(PWD):/s $(BASE_IMAGE) \
	sh -c "cd /s && go get && go mod tidy"

format:
	docker run --rm -it -v $(PWD):/s $(BASE_IMAGE) \
	sh -c "cd /s && find . -type f -name '*.go' | xargs gofmt -l -w -s"

test: test-example test-command test-sys

test-nodocker: test-example-nodocker test-command-nodocker test-sys-nodocker

define DOCKERFILE_TEST_EXAMPLE
FROM $(BASE_IMAGE)
RUN apk add --no-cache make
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY Makefile *.go ./
COPY example ./example
endef
export DOCKERFILE_TEST_EXAMPLE

test-example:
	echo "$$DOCKERFILE_TEST_EXAMPLE" | docker build -q . -f - -t dctoolkit-test-example >/dev/null
	docker run --rm -it dctoolkit-test-example make test-example-nodocker

test-example-nodocker:
	$(eval export CGO_ENABLED = 0)
	$(foreach f,$(shell echo example/*),go build -o /dev/null ./$(f)$(NL))

define DOCKERFILE_TEST_COMMAND
FROM $(BASE_IMAGE)
WORKDIR /s
RUN apk add --no-cache make
COPY go.mod go.sum ./
RUN go mod download
COPY Makefile *.go ./
COPY cmd ./cmd
endef
export DOCKERFILE_TEST_COMMAND

test-command:
	echo "$$DOCKERFILE_TEST_COMMAND" | docker build -q . -f - -t dctoolkit-test-command >/dev/null
	docker run --rm -it dctoolkit-test-command make test-command-nodocker

test-command-nodocker:
	$(eval export CGO_ENABLED = 0)
	$(foreach d,$(shell echo cmd/*/),go build -o /dev/null ./$(d)$(NL))

define DOCKERFILE_TEST_SYS
FROM $(BASE_IMAGE)
RUN apk add --no-cache make docker-cli
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY Makefile *.go ./
COPY test-sys ./test-sys
endef
export DOCKERFILE_TEST_SYS

test-sys:
	echo "$$DOCKERFILE_TEST_SYS" | docker build -q . -f - -t dctk-test-sys
	docker run --rm -it \
	--name dctk-test-sys \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	dctk-test-sys \
	make test-sys-nodocker

HUBS = $(shell echo test-sys/*/ | xargs -n1 basename)

test-sys-nodocker:
	$(foreach HUB,$(HUBS),docker build -q test-sys/$(HUB) -t dctk-test-sys-hub-$(HUB)$(NL))
	$(eval export CGO_ENABLED = 0)
	go test -v ./test-sys

run-example:
	@test -f "./example/$(E).go" || ( echo "example file not found"; exit 1 )
	docker run --rm -it -v $(PWD):/s \
	--network=host \
	$(BASE_IMAGE) sh -c "\
	cd /s && go run example/$(E).go"

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
