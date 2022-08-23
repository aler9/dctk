test-manual:
	cd ./test-manual && docker-compose up \
	--build \
	--renew-anon-volumes \
	--force-recreate
