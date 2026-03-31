#!/bin/sh
set -e
# 与 docker-compose 中 /app/data 挂载一致；绑定卷常见为 root 属主，需纠正否则业务进程会回退到临时目录导致重启丢文件
DATA_ROOT="${LAN_IM_DATA_DIR:-/app/data}"
mkdir -p "$DATA_ROOT/uploads" "$DATA_ROOT/temp_chunks"
if [ "$(id -u)" = "0" ]; then
	chown -R imuser:nobody "$DATA_ROOT" || true
	exec su-exec imuser:nobody "$@"
fi
exec "$@"
