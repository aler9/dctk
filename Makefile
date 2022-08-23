BASE_IMAGE = golang:1.18-alpine3.15
LINT_IMAGE = golangci/golangci-lint:v1.45.2

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

include scripts/*.mk
