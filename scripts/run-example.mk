run-example:
	@test -f "./examples/$(E).go" || ( echo "example file not found"; exit 1 )
	docker run --rm -it -v $(PWD):/s -w /s \
	--network=host \
	$(BASE_IMAGE) \
	sh -c "go run examples/$(E).go"
