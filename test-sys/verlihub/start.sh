#!/bin/sh

mysqld_safe &

# wait for mysql
while true; do
    sleep 1
    [ -S /run/mysqld/mysqld.sock ] && break
done

case "$1" in
ConnNoIp)
    echo "UPDATE SetupList SET val = '0' WHERE var = 'send_user_ip';" |  mysql -D verlihub
    ;;

ConnCompression)
    echo "UPDATE SetupList SET val = '10' WHERE var = 'zlib_min_len';" | mysql -D verlihub
    echo "UPDATE SetupList SET val = '0' WHERE var = 'disable_zlib';" | mysql -D verlihub
    ;;
esac

#echo "select * from reglist;" | mysql -D verlihub
#echo "SELECT * from SetupList;" | mysql -D verlihub

verlihub &
VPID=$!

wait
