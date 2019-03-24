
help:
	@echo "usage: make [action] [args...]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy                    run go mod tidy."
	@echo "  format                      format source files."
	@echo "  test [proto] [testname]     run available tests. by default all"
	@echo "                              tests are run, but they can be"
	@echo "                              filtered by protocol or name."
	@echo "                              add V=1 to increase verbosity."
	@echo "  run-example [name]          run an example by name."
	@echo "  run-command '[name] [args]' run a command by name."
	@echo ""

# do not treat arguments as targets
%:
	@[ "$(word 1, $(MAKECMDGOALS))" != "$@" ] || { echo "unrecognized command."; exit 1; }

ARGS := $(wordlist 2, $(words $(MAKECMDGOALS)), $(MAKECMDGOALS))

mod-tidy:
	docker run --rm -it -v $(PWD):/src amd64/golang:1.11 \
		sh -c "cd /src && go get -m ./... && go mod tidy"

format:
	@docker run --rm -it \
		-v $(PWD):/src \
		amd64/golang:1.11-stretch \
		sh -c "cd /src \
		&& find . -type f -name '*.go' | xargs gofmt -l -w -s"

.PHONY: test
test:
	$(eval PROTOCOLS := $(if $(word 1, $(ARGS)), $(word 1, $(ARGS)), nmdc adc))
	$(eval TESTNAMES := $(if $(word 2, $(ARGS)), $(word 2, $(ARGS)), $(shell cd test && ls -v *.go | sed 's/\.go$$//')))
	$(eval OUT := $(if $(V), /dev/stdout, /dev/null))

	@docker container kill dctk-hub dctk-test >/dev/null 2>&1 || exit 0
	@docker container rm dctk-hub dctk-test >/dev/null 2>&1 || exit 0
	@docker network rm dctk-test >/dev/null 2>&1 || exit 0

	@echo "building main test image..."
	@docker build . -f test/Dockerfile -t dctk-test >$(OUT)
	@docker network create dctk-test >/dev/null
	@for PROTO in $(PROTOCOLS); do \
		case $$PROTO in \
			nmdc) echo "building nmdc test image..."; docker build test/verlihub -t dctk-hub >$(OUT); HUBURL="nmdc://dctk-hub:4111";; \
			adc) echo "building adc test image..."; docker build test/luadch -t dctk-hub >$(OUT); HUBURL="adcs://dctk-hub:5001";; \
			*) echo "protocol unrecognized"; exit 1;; \
		esac; \
		\
		for TESTNAME in $(TESTNAMES); do \
			[ -f test/$$TESTNAME.go ] || { echo "test not found"; exit 1; }; \
			echo "[$$PROTO $$TESTNAME]"; \
			docker run --rm -d --network=dctk-test --name=dctk-hub \
				dctk-hub $$TESTNAME >/dev/null; \
			docker run --rm -it --network=dctk-test --name=dctk-test \
				-v $$PWD:/src \
				-e HUBURL=$$HUBURL \
				-e TESTNAME=$$TESTNAME \
				-e PROTO=$$PROTO \
				dctk-test >$(OUT); \
				[ "$$?" -eq 0 ] && echo "SUCCESS" || echo "FAIL"; \
			docker container kill dctk-hub >/dev/null 2>&1; \
		done \
	done
	@docker network rm dctk-test >/dev/null 2>&1 || exit 0

run-example:
	$(eval EXAMPLE := $(word 1, $(ARGS)))
	@test -f "./example/$(EXAMPLE).go" || ( echo "example file not found"; exit 1 )
	@docker run --rm -it \
		-v $(PWD):/src \
		--network=host \
		amd64/golang:1.11-stretch sh -c "\
		cd /src && go run example/$(EXAMPLE).go"

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
		dctk-runcmd $(ARGS)
