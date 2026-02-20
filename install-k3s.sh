#!/usr/bin/env bash
# Minimal installer for Terraform and k3s (clean, no logs)
# Displays immediately "debut du script"

printf '%s\n' "debut du script"
set -euo pipefail
umask 077

CONFIG_FILE="${CONFIG_FILE:-./install-config.yaml}"
CURL_OPTS=(--fail --silent --show-error --location --max-time 30 --proto '=https')

err(){ printf 'ERREUR: %s\n' "$*" >&2; }
abort(){ err "$*"; exit 1; }

require_root(){ [[ $EUID -eq 0 ]] || abort "ce script doit être exécuté en root (sudo)."; }
check_arch(){ case "$(uname -m)" in x86_64|amd64) return 0 ;; *) abort "architecture non supportée: $(uname -m)" ;; esac }

parse_components(){
  [[ -f "$CONFIG_FILE" ]] || abort "fichier de config introuvable: $CONFIG_FILE"
  local parse
  parse=$(awk '
    BEGIN{in=0}
    /^\s*components\s*:/ { in=1; next }
    in {
      if (match($0,/^\s*-\s*([a-zA-Z0-9_-]+)\s*:/,m)) { print "COMP:" m[1]; next }
      if (match($0,/^\s{2,}([a-zA-Z0-9._-]+)\s*:\s*(.*)$/,m)) { k=m[1]; v=m[2]; gsub(/^\s*[\t]+|[\t]+$/,"",v); print "  " k ":" v; next }
      if (in && match($0,/^[^[:space:]]/) ) exit
    }
  ' "$CONFIG_FILE")

  components_list=()
  declare -gA comp_props
  local current=""
  while IFS= read -r line; do
    [[ -z "${line:-}" ]] && continue
    if [[ "$line" == COMP:* ]]; then
      current="${line#COMP:}"
      components_list+=("$current")
      continue
    fi
    if [[ "$line" =~ ^[[:space:]]+([a-zA-Z0-9._-]+):(.*)$ ]]; then
      local k="${BASH_REMATCH[1]}" v="${BASH_REMATCH[2]}"
      # trim
      v="${v#"${v%%[![:space:]]*}"}"
      v="${v%"${v##*[![:space:]]}"}