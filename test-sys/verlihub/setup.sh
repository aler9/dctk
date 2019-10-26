#!/bin/sh

mysqld_safe &
MPID=$!

# wait for mysql
while true; do
    sleep 1
    [ -S /run/mysqld/mysqld.sock ] && break
done

echo "CREATE DATABASE verlihub;" | mysql
echo "CREATE USER verlihub@localhost IDENTIFIED BY 'verlihub';" | mysql
echo "GRANT ALL PRIVILEGES ON verlihub.* TO verlihub@localhost;" | mysql

# setup verlihub
cp -r /usr/local/share/verlihub/config/ /etc/verlihub/
cat > /etc/verlihub/dbconfig << EOF
db_host = localhost
db_data = verlihub
db_user = verlihub
db_pass = verlihub
EOF

# run verlihub to create tables
verlihub &
VPID=$!

# wait for verlihub
while true; do
    nc -z -v -w1 localhost 4111 2>/dev/null && break
done

kill -TERM $VPID
wait $VPID

echo "UPDATE SetupList SET val = '1' WHERE var = 'send_user_ip';" | mysql -D verlihub
echo "UPDATE SetupList SET val = 'test topic' WHERE var = 'hub_topic';" | mysql -D verlihub
echo "UPDATE SetupList SET val = '10' WHERE var = 'search_number';" | mysql -D verlihub
echo "INSERT INTO reglist (nick, class, reg_date, reg_op, pwd_change, pwd_crypt, login_pwd) VALUES \
    ('testdctk_auth', 3, UNIX_TIMESTAMP(NOW()), 'installation', 0, 0, 'testpa&#36;ss');" \
    | mysql -D verlihub

kill -TERM $MPID
wait
