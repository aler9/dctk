
help:
	@echo "usage: make [action] [args...]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy                      run go mod tidy"
	@echo "  format                        format source files"
	@echo "  test P=[proto] T=[testname]   run available tests. tests can be"
	@echo "                                filtered by protocol or name."
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
		amd64/golang:1.11 \
		sh -c "cd /src && go get -m ./... && go mod tidy"

format:
	@docker run --rm -it \
		-v $(PWD):/src \
		amd64/golang:1.11-stretch \
		sh -c "cd /src \
		&& find . -type f -name '*.go' | xargs gofmt -l -w -s"

test-example:
	echo "FROM amd64/golang:1.11-stretch \n\
		WORKDIR /src \n\
		COPY go.mod go.sum ./ \n\
		RUN go mod download \n\
		COPY Makefile *.go ./ \n\
		COPY example ./example" | docker build . -f - -t dctoolkit-test-example >/dev/null
	docker run --rm -it dctoolkit-test-example make test-example-nodocker

test-example-nodocker:
	$(foreach f, $(wildcard example/*), go build -o /dev/null $(f)$(NL))

define TEST_LIB_INIT
@docker container kill $$(docker container ps -a -q --filter='name=dctk-*') \
	>/dev/null 2>&1 || exit 0
docker build . -f test/Dockerfile -t dctk-test >$(OUT)
docker network create dctk-test >/dev/null 2>&1 || exit 0
endef

define TEST_LIB_CLEANUP
@docker network rm dctk-test >/dev/null 2>&1 || exit 0
endef

define TEST_UNIT
@[ -f test/$(TNAME).go ] || { echo "test not found"; exit 1; }
@echo "testing $(PROTO) -> $(TNAME)"
@docker run --rm -d --network=dctk-test --name=dctk-hub-$(PROTO)-$(TNAME) \
	dctk-hub $(TNAME) >/dev/null
@docker run --rm -it --network=dctk-test --name=dctk-test \
	-v $(PWD):/src \
	-e HUBURL=$(subst addr,dctk-hub-$(PROTO)-$(TNAME),$(HUBURL)) \
	-e TNAME=$(TNAME) \
	-e PROTO=$(PROTO) \
	dctk-test >$(OUT)
@docker container kill dctk-hub-$(PROTO)-$(TNAME) >/dev/null 2>&1
endef

define TEST_LIB_PROTO_nmdc
docker build test/verlihub -t dctk-hub >$(OUT)
$(eval HUBURL = "nmdc://addr:4111")
$(foreach TNAME, $(TNAMES), $(call TEST_UNIT)$(NL))
endef

define TEST_LIB_PROTO_adc
docker build test/luadch -t dctk-hub >$(OUT)
$(eval HUBURL = "adcs://addr:5001")
$(foreach TNAME, $(TNAMES), $(call TEST_UNIT)$(NL))
endef

test-lib:
	$(eval PROTOCOLS := $(if $(P), $(P), example nmdc adc))
	$(eval TNAMES := $(if $(T), $(T), $(shell cd test && ls -v *.go | sed 's/\.go$$//')))
	$(eval OUT := $(if $(V), /dev/stdout, /dev/null))
	$(TEST_LIB_INIT)
	$(foreach PROTO, $(PROTOCOLS), $(call TEST_LIB_PROTO_$(PROTO))$(NL))
	$(TEST_LIB_CLEANUP)

.PHONY: test
test: test-example test-lib

run-example:
	@test -f "./example/$(E).go" || ( echo "example file not found"; exit 1 )
	@docker run --rm -it \
		-v $(PWD):/src \
		--network=host \
		amd64/golang:1.11-stretch sh -c "\
		cd /src && go run example/$(E).go"

run-command:
	@echo "FROM amd64/golang:1.11-stretch \n\
		WORKDIR /src \n\
		COPY go.mod go.sum ./ \n\
		RUN go mod download \n\
		COPY . ./ \n\
		RUN go install ./..." | docker build . -q -f - -t dctk-runcmd
	@docker run --rm -it \
		--network=host \
		-e COLUMNS=$(shell tput cols) \
		dctk-runcmd $(N) $(A)
