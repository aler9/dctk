
.PHONY: help
help:
	@echo "usage: make [action] [args...]"
	@echo " "
	@echo "available actions:"
	@echo " "
	@echo "  format                      format source files."
	@echo " "
	@echo "  test [proto] [testname]     run available tests. By default all"
	@echo "                              tests are run, but they can be"
	@echo "                              filtered by protocol or name."
	@echo "                              add V=1 to increase verbosity."
	@echo " "
	@echo "  run-example [name]          run an example by name."
	@echo " "
	@echo "  run-command '[name] [args]' run a command by name."
	@echo " "


.PHONY: format
format:
	@docker run --rm -it \
		-v $(PWD):/src \
		amd64/golang:1.11-stretch \
		bash -c "cd /src \
		&& find . -type f -name '*.go' | xargs gofmt -l -w -s"


.PHONY: test
ifeq (test, $(firstword $(MAKECMDGOALS)))
  ARGS := $(wordlist 2, $(words $(MAKECMDGOALS)), $(MAKECMDGOALS))
  $(eval $(ARGS):;@:)
endif
test: PROTOCOLS := $(if $(word 1, $(ARGS)), $(word 1, $(ARGS)), nmdc adc)
test: TESTNAMES := $(if $(word 2, $(ARGS)), $(word 2, $(ARGS)), $(shell cd test && ls -v *.go | sed 's/\.go$$//'))
test: OUT := $(if $(V), /dev/stdout, /dev/null)
test:
  # cleanup residuals
	@docker container kill dctk-hub dctk-test >/dev/null 2>&1 || exit 0
	@docker network rm dctk-test >/dev/null 2>&1 || exit 0

  # run tests
	@echo "building main image..."
	@docker build . -f test/Dockerfile -t dctk-test >$(OUT)
	@docker network create dctk-test >/dev/null
	@for PROTO in $(PROTOCOLS); do \
		case $$PROTO in \
			nmdc) echo "building nmdc image..."; docker build test/verlihub -t dctk-hub >$(OUT); HUBURL="nmdc://dctk-hub:4111";; \
			adc) echo "building adc image..."; docker build test/luadch -t dctk-hub >$(OUT); HUBURL="adcs://dctk-hub:5001";; \
			*) echo "protocol unrecognized"; exit 1;; \
		esac; \
		\
		for TESTNAME in $(TESTNAMES); do \
			[ -f test/$$TESTNAME.go ] || { echo "test not found"; exit 1; }; \
			echo "[$$PROTO $$TESTNAME]"; \
			# start hub \
			docker run --rm -d --network=dctk-test --name=dctk-hub \
				dctk-hub $1 >/dev/null; \
			# run test \
			docker run --rm -it --network=dctk-test --name=dctk-test \
				-v $$PWD:/src -e HUBURL=$$HUBURL -e TEST=$$TESTNAME dctk-test >$(OUT); \
				[ "$$?" -eq 0 ] && echo "SUCCESS" || echo "FAILED"; \
			# stop hub \
			docker container kill dctk-hub >/dev/null 2>&1; \
			\
		done \
	done
	@docker network rm dctk-test >/dev/null 2>&1 || exit 0


.PHONY: run-example
ifeq (run-example, $(firstword $(MAKECMDGOALS)))
  ARGS := $(wordlist 2, $(words $(MAKECMDGOALS)), $(MAKECMDGOALS))
  $(eval $(ARGS):;@:)
endif
run-example: EXAMPLE := $(word 1, $(ARGS))
run-example:
	@test -f "./example/$(EXAMPLE).go" || ( echo "example file not found"; exit 1 )
	@docker run --rm -it \
		-v $(PWD):/src \
		-p 3009:3009 \
		-p 3009:3009/udp \
		-p 3010:3010 \
		amd64/golang:1.11-stretch bash -c "\
		cd /src && go run example/$(EXAMPLE).go"


.PHONY: run-command
ifeq (run-command, $(firstword $(MAKECMDGOALS)))
  ARGS := $(wordlist 2, $(words $(MAKECMDGOALS)), $(MAKECMDGOALS))
  $(eval $(ARGS):;@:)
endif
define COMMAND_DOCKERFILE
FROM amd64/golang:1.11-stretch
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go install ./...
endef
export COMMAND_DOCKERFILE
run-command:
	@echo "$$COMMAND_DOCKERFILE" | docker build . -f - -t dctk-runcmd >/dev/null
	@docker run --rm -it -e COLUMNS=$(shell tput cols) dctk-runcmd $(ARGS)
