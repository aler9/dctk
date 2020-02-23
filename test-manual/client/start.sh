#!/bin/sh -e

while true; do
	echo "waiting"
	nc -z -v -w1 127.0.0.1 4111 2>/dev/null && echo "ok" && break
	sleep 1
done

exec /out
