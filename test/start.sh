#!/bin/sh

# wait for verlihub
while true; do
	nc -z -v -w1 gotk-verlihub 4111 2>/dev/null && break
done

go run test/$1.go
