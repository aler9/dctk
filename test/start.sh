#!/bin/sh

# wait for hub
while true; do
    nc -z -v -w1 dctk-hub ${HUBURL##*:} 2>/dev/null && break
    #sleep 1
done

go run test/$TEST.go
