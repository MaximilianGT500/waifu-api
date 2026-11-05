#!/bin/sh
set -eu

CONF=/etc/nginx/conf.d/00-client-ip.conf
DEFAULT_TRUSTED="127.0.0.0/8 ::1/128 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7"

trusted=$(printf '%s' "${TRUSTED_PROXIES:-}" | tr ',;' '  ')
[ -n "$(printf '%s' "$trusted" | tr -d ' \t\r\n')" ] || trusted=$DEFAULT_TRUSTED

header=$(printf '%s' "${REAL_IP_HEADER:-}" | tr -cd 'A-Za-z0-9-')
[ -n "$header" ] || header=X-Forwarded-For

{
  for entry in $trusted; do
    case "$entry" in
      none|None|NONE) ;;
      *[!0-9A-Fa-f.:/]*) echo "gateway: ignoring invalid TRUSTED_PROXIES entry '$entry'" >&2 ;;
      *) printf 'set_real_ip_from %s;\n' "$entry" ;;
    esac
  done
  printf 'real_ip_header %s;\nreal_ip_recursive on;\n' "$header"
  case "$(printf '%s' "$header" | tr 'A-Z' 'a-z')" in
    cf-connecting-ip) printf 'map $http_cf_connecting_ip $cf_connecting_ip_hint { default ""; }\n' ;;
    *) printf 'map $http_cf_connecting_ip $cf_connecting_ip_hint { default $http_cf_connecting_ip; }\n' ;;
  esac
} > "$CONF"

exec /docker-entrypoint.sh "$@"
