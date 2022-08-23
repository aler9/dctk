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
