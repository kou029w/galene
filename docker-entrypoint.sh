#!/bin/sh
# Generate Galène configuration from environment variables, then start the
# server.  Intended as the container ENTRYPOINT so that everything lives under
# the single /data volume.
#
#   GALENE_PROXY_URL          -> /data/config.json  ".proxyURL"
#   GALENE_ADMIN_USERNAME     -> /data/config.json admin user (with the
#   GALENE_ADMIN_PASSWORD        password below and the "admin" permission)
#   GALENE_GROUP_<NAME>       -> /data/groups/<name>.json (whole file, JSON;
#                                <NAME> is lowercased, e.g. PUBLIC -> public.json)
#   GALENE_GROUP_<NAME>_USERS -> the ".users" field of /data/groups/<name>.json
#   GALENE_TURN               -> passed as the -turn command-line option
#
# config.json is merged (so each variable is independent); group files named by
# GALENE_GROUP_<NAME> are overwritten.
set -eu

DATA_DIR=/data
CONFIG="$DATA_DIR/config.json"
GROUPS_DIR="$DATA_DIR/groups"

mkdir -p "$GROUPS_DIR" "$DATA_DIR/recordings"

[ -f "$CONFIG" ] || echo '{}' >"$CONFIG"

if [ -n "${GALENE_PROXY_URL:-}" ]; then
	jq --arg v "$GALENE_PROXY_URL" '.proxyURL = $v' "$CONFIG" >"$CONFIG.tmp"
	mv "$CONFIG.tmp" "$CONFIG"
	echo "entrypoint: set proxyURL in $CONFIG"
fi

if [ -n "${GALENE_ADMIN_USERNAME:-}" ]; then
	jq --arg u "$GALENE_ADMIN_USERNAME" --arg p "${GALENE_ADMIN_PASSWORD:-}" \
		'.writableGroups = true | .users[$u] = {password: $p, permissions: "admin"}' \
		"$CONFIG" >"$CONFIG.tmp"
	mv "$CONFIG.tmp" "$CONFIG"
	echo "entrypoint: set admin user '$GALENE_ADMIN_USERNAME' in $CONFIG"
fi

for var in $(env | sed -n 's/^\(GALENE_GROUP_[A-Za-z0-9_]*\)=.*/\1/p'); do
	case "$var" in
	*_USERS) continue ;;
	esac
	name=${var#GALENE_GROUP_}
	[ -n "$name" ] || continue
	name=$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')
	printenv "$var" | jq . >"$GROUPS_DIR/$name.json"
	echo "entrypoint: wrote group '$name'"
done

for var in $(env | sed -n 's/^\(GALENE_GROUP_[A-Za-z0-9_]*_USERS\)=.*/\1/p'); do
	name=${var#GALENE_GROUP_}
	name=${name%_USERS}
	[ -n "$name" ] || continue
	name=$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')
	file="$GROUPS_DIR/$name.json"
	[ -f "$file" ] || echo '{}' >"$file"
	users=$(printenv "$var")
	jq --argjson users "$users" '.users = $users' "$file" >"$file.tmp"
	mv "$file.tmp" "$file"
	echo "entrypoint: set users of group '$name'"
done

if [ -n "${GALENE_TURN:-}" ]; then
	set -- "$@" -turn "$GALENE_TURN"
	echo "entrypoint: set -turn '$GALENE_TURN'"
fi

exec galene "$@"
