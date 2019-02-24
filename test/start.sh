#!/bin/sh

# wait for hub
while true; do
    nc -z -v -w1 dctk-hub ${HUBURL##*:} 2>/dev/null && break
done

go run test/$TEST.go
