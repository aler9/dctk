
.PHONY: $(shell ls)

BASE_IMAGE = amd64/golang:1.13-alpine3.10

help:
	@echo "usage: make [action]"
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

define DOCKERFILE_TEST
FROM $(BASE_IMAGE)
RUN apk add --no-cache make docker-cli
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
endef
export DOCKERFILE_TEST

test:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t dctk-test
	docker run --rm -it \
	--name dctk-test \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	dctk-test \
	make test-nodocker

test-nodocker:
	$(eval export CGO_ENABLED = 0)
	$(foreach f,$(shell echo example/*),go build -o /dev/null ./$(f)$(NL))
	$(foreach d,$(shell echo cmd/*/),go build -o /dev/null ./$(d)$(NL))
	$(foreach HUB,$(shell echo test/*/ | xargs -n1 basename), \
	docker build -q test/$(HUB) -t dctk-test-hub-$(HUB)$(NL))
	go test -v ./test

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
