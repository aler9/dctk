
BASE_IMAGE = amd64/golang:1.11-stretch

help:
	@echo "usage: make [action] [args...]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy                      run go mod tidy"
	@echo "  format                        format source files"
	@echo "  test H=[hub] U=[unit]         run available tests. tests can be"
	@echo "                                filtered by hub or unit."
	@echo "                                add V=1 to increase verbosity"
	@echo "  run-example E=[name]          run an example by name"
	@echo "  run-command N=[name] A=[args] run a command by name"
	@echo ""

blank :=
define NL

$(blank)
endef

mod-tidy:
	docker run --rm -it -v $(PWD):/src \
	$(BASE_IMAGE) \
	sh -c "cd /src && go get -m ./... && go mod tidy"

format:
	@docker run --rm -it -v $(PWD):/src \
	$(BASE_IMAGE) \
	sh -c "cd /src \
	&& find . -type f -name '*.go' | xargs gofmt -l -w -s"

.PHONY: test
test: test-example test-command test-lib

test-example:
	echo "FROM $(BASE_IMAGE) \n\
	WORKDIR /src \n\
	COPY go.mod go.sum ./ \n\
	RUN go mod download \n\
	COPY Makefile *.go ./ \n\
	COPY example ./example" | docker build . -f - -t dctoolkit-test-example >/dev/null
	docker run --rm -it dctoolkit-test-example make test-example-nodocker

test-example-nodocker:
	$(foreach f,$(shell echo example/*),go build -o /dev/null ./$(f)$(NL))

test-command:
	echo "FROM $(BASE_IMAGE) \n\
	WORKDIR /src \n\
	COPY go.mod go.sum ./ \n\
	RUN go mod download \n\
	COPY Makefile *.go ./ \n\
	COPY cmd ./cmd" | docker build . -f - -t dctoolkit-test-command >/dev/null
	docker run --rm -it dctoolkit-test-command make test-command-nodocker

test-command-nodocker:
	$(foreach d,$(shell echo cmd/*/),go build -o /dev/null ./$(d)$(NL))

define TEST_LIB_UNIT
@[ -f test/$(UNIT).go ] || { echo "test not found"; exit 1; }
@echo "testing $(HUB) -> $(UNIT)"
@docker run --rm -d --network=dctk-test --name=dctk-hub-$(HUB)-$(UNIT) \
dctk-hub-$(HUB) $(UNIT) >/dev/null
@docker run --rm -it --network=dctk-test --name=dctk-test \
-v $(PWD):/src \
-e HUBURL=$(subst addr,dctk-hub-$(HUB)-$(UNIT),$(shell cat test/$(HUB)/URL)) \
-e UNIT=$(UNIT) \
dctk-unit >$(OUT)
@docker container kill dctk-hub-$(HUB)-$(UNIT) >/dev/null 2>&1
endef

test-lib:
	$(eval HUBS := $(if $(H), $(H), $(shell echo test/*/ | xargs -n1 basename)))
	$(eval UNITS := $(if $(U), $(U), $(shell echo test/*.go | xargs -n1 basename | sed 's/\.go$$//')))
	$(eval OUT := $(if $(V), /dev/stdout, /dev/null))

  # cleanup
	@docker container kill $$(docker ps -a -q --filter='name=dctk-*') >/dev/null 2>&1 || exit 0
	docker network rm dctk-test >/dev/null 2>&1 || exit 0

  # build images
	docker build . -f test/Dockerfile -t dctk-unit >$(OUT)
	$(foreach HUB,$(HUBS),docker build test/$(HUB) -t dctk-hub-$(HUB) >$(OUT)$(NL))

  # run units
	docker network create dctk-test >/dev/null
	$(foreach HUB,$(HUBS),$(foreach UNIT,$(UNITS),$(TEST_LIB_UNIT)$(NL))$(NL))
	docker network rm dctk-test

run-example:
	@test -f "./example/$(E).go" || ( echo "example file not found"; exit 1 )
	@docker run --rm -it -v $(PWD):/src \
	--network=host \
	$(BASE_IMAGE) sh -c "\
	cd /src && go run example/$(E).go"

run-command:
	@echo "FROM $(BASE_IMAGE) \n\
	WORKDIR /src \n\
	COPY go.mod go.sum ./ \n\
	RUN go mod download \n\
	COPY . ./ \n\
	RUN go install ./..." | docker build . -q -f - -t dctk-runcmd
	@docker run --rm -it \
	--network=host \
	-e COLUMNS=$(shell tput cols) \
	dctk-runcmd $(N) $(A)
