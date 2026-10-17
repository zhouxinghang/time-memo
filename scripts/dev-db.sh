#!/usr/bin/env bash
# Local development only. Production should use authenticated PostgreSQL / Compose.
set -euo pipefail
cd "$(dirname "$0")/.."
project_dir="$PWD"
if [[ -n "${PG_BIN:-}" ]]; then
  pg_bin="$PG_BIN"
elif command -v pg_config >/dev/null 2>&1; then
  pg_bin="$(pg_config --bindir)"
else
  echo '请先安装 PostgreSQL，或设置 PG_BIN 指向其 bin 目录。' >&2
  exit 1
fi
pg_data="$project_dir/data/postgres"
pg_port="${DEV_PG_PORT:-55432}"
if [[ ! "$pg_port" =~ ^[0-9]+$ ]] || (( pg_port < 1 || pg_port > 65535 )); then
  echo 'DEV_PG_PORT 须为 1–65535。' >&2
  exit 1
fi
action="${1:-start}"
if [[ "$action" == stop ]]; then
  "$pg_bin/pg_ctl" -D "$pg_data" stop -m fast
  exit 0
fi
if [[ "$action" != start ]]; then
  echo '用法：bash scripts/dev-db.sh [start|stop]' >&2
  exit 1
fi
mkdir -p "$project_dir/data"
if [[ ! -f "$pg_data/PG_VERSION" ]]; then
  "$pg_bin/initdb" -D "$pg_data" -A trust --no-locale -E UTF8
fi
if ! "$pg_bin/pg_ctl" -D "$pg_data" status >/dev/null 2>&1; then
  "$pg_bin/pg_ctl" -D "$pg_data" -l "$project_dir/data/postgres.log" -o "-p $pg_port -h 127.0.0.1 -k /tmp" start
fi
db_user="$(id -un)"
if [[ "$("$pg_bin/psql" -h 127.0.0.1 -p "$pg_port" -U "$db_user" -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname='timememo'")" != 1 ]]; then
  "$pg_bin/createdb" -h 127.0.0.1 -p "$pg_port" -U "$db_user" timememo
fi
echo "本机开发数据库已就绪（仅监听 127.0.0.1，无密码信任认证，勿用于部署）。"
echo "DATABASE_URL=postgres://$db_user@127.0.0.1:$pg_port/timememo?sslmode=disable"
