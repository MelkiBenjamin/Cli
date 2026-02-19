#!/usr/bin/env bash
# Minimal, modular installer with checksum verification for Terraform and k3s.
# Adds detailed logs for start/progress/end and command outputs.

# Affiche immédiatement le marqueur demandé (visible même si le script plante après)
printf '%s\n' "debut du script"

set -euo pipefail
umask 077

CONFIG_FILE="[0m{CONFIG_FILE:-./install-config.yaml}"
CURL_OPTS=(--fail --silent --show-error --location --max-time 30 --proto '=https')

# Logging helpers (timestamped)
err(){ printf '%s ERREUR: %s\n' "$(date -u +%FT%TZ)" "$*" >&2; }
log(){ printf '%s INFO: %s\n' "$(date -u +%FT%TZ)" "$*"; }
abort(){ err "$*"; exit 1; }
start_step(){ log "START: $*"; }
end_step(){ log "END: $*"; }

# Run a command, capture and print its output (timestamped), preserve exit code
run_and_log(){
  local tmp rc
  tmp=$(mktemp) || { err "mktemp failed"; return 1; }
  log "CMD START: $*"
  if "$@" >"$tmp" 2>&1; then
    sed "s/^/$(date -u +%FT%TZ) OUT: /" "$tmp"
    rc=0
    log "CMD OK: $*"
  else
    sed "s/^/$(date -u +%FT%TZ) OUT: /" "$tmp"
    rc=1
    err "CMD FAIL: $*"
  fi
  rm -f "$tmp"
  return $rc
}

require_root(){ [[ $EUID -eq 0 ]] || abort "ce script doit être exécuté en root (sudo)."; }
check_arch(){ case "$(uname -m)" in x86_64|amd64) return 0 ;; *) abort "architecture non supportée: $(uname -m)" ;; esac }

# --- YAML parse (strict, minimal) ---
parse_components(){
  start_step "parse_components"
  [[ -f "$CONFIG_FILE" ]] || abort "fichier de config introuvable: $CONFIG_FILE"
  local parse
  parse=$(awk '
    BEGIN{in=0}
    /^\s*components\s*:/{ in=1; next }
    in {
      if (match($0,/^\s*-\s*([a-zA-Z0-9_-]+)\s*:/,m)) { print "COMP:" m[1]; next }
      if (match($0,/^\s{2,}([a-zA-Z0-9._-]+)\s*:\s*(.*)$/,m)) { k=m[1]; v=m[2]; gsub(/^[ \t]+|[ \t]+$/,"",v); print "  " k ":" v; next }
      if (in && match($0,/^[^[:space:]]/)) exit
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
      # trim leading/trailing spaces
      v="${v#"${v%%[![:space:]]*}"}"
      v="${v%"${v##*[![:space:]]}"}"
      # remove surrounding double or single quotes if present
      if [[ "[0m{v:0:1}" == '"' && "${v: -1}" == '"' ]]; then v="${v:1:-1}"; fi
      if [[ "[0m{v:0:1}" == "'" && "${v: -1}" == "'" ]]; then v="${v:1:-1}"; fi
      comp_props["${current}.${k}"]="$v"
    fi
  done <<< "$parse"
  end_step "parse_components"
}

contains_component(){ local want="$1"; for c in "${components_list[@]:-}"; do [[ "$c" == "$want" ]] && return 0; done; return 1 }

# Download + verify helpers
sha256_of_file(){ sha256sum "$1" | awk '{print $1}'; }

verify_checksum_from_sums(){
  # args: <file> <sums-file> <needle-filename>
  local file="$1" sums="$2" needle="$3"
  [[ -f "$sums" ]] || return 1
  local expected actual
  expected=$(grep -E "[[:space:]]${needle}$" "$sums" | awk '{print $1}' | head -1) || true
  log "Expected sha256 for ${needle}: ${expected:-<none>}"
  actual=$(sha256_of_file "$file") || { err "sha256_of_file failed"; return 1; }
  log "Actual sha256 for ${file}: ${actual:-<none>}"
  [[ "$actual" == "$expected" ]]
}

download(){
  local url="$1" dest="$2"
  log "DL START: $url -> $dest"
  if run_and_log curl "${CURL_OPTS[@]}" -o "$dest" "$url"; then
    run_and_log chmod 0700 "$dest" || true
    log "DL OK: $url -> $dest"
    return 0
  else
    rm -f "$dest"
    log "DL FAIL: $url"
    return 1
  fi
}

install_terraform(){
  start_step "install_terraform"
  local version="$1" arch=amd64 fname="terraform_${version}_linux_${arch}.zip"
  local base="https://releases.hashicorp.com/terraform/${version}"
  local zip="/tmp/${fname}" sums="/tmp/terraform_${version}_SHA256SUMS"

  log "Terraform: download ${version}"
  download "${base}/${fname}" "$zip" || abort "échec téléchargement Terraform"
  download "${base}/terraform_${version}_SHA256SUMS" "$sums" || abort "échec téléchargement des checksums Terraform"

  log "Terraform: verify checksum"
  if ! verify_checksum_from_sums "$zip" "$sums" "$fname"; then
    err "checksum Terraform invalide"
    abort "checksum Terraform invalide"
  fi

  log "Terraform: unzip/install"
  run_and_log unzip -o "$zip" -d /usr/local/bin || abort "unzip échec"
  run_and_log chmod 0755 /usr/local/bin/terraform || true
  rm -f "$zip" "$sums"
  log "Terraform ${version} installé dans /usr/local/bin/terraform"
  end_step "install_terraform"
}

install_k3s(){
  start_step "install_k3s"
  local version="$1"
  local tag="${version}" # expect tag like v1.28.0+k3s1 or user provided
  local base="https://github.com/k3s-io/k3s/releases/download/${tag}"
  local bin_tmp="/tmp/k3s-${tag}-amd64" sums_tmp="/tmp/sha256-${tag}.txt"

  log "k3s: try download checksums for tag ${tag}"
  local sums_urls=("${base}/sha256sum-amd64.txt" "${base}/sha256sums.txt" "${base}/sha256sum.txt")
  local sums_url found_sums
  for sums_url in "${sums_urls[@]}"; do
    if download "$sums_url" "$sums_tmp"; then found_sums=1; break; fi
  done
  [[ -n "${found_sums}" ]] || abort "Impossible de récupérer le fichier de checksums k3s; fournir un tag de release valide"

  log "k3s: download binary"
  download "${base}/k3s" "$bin_tmp" || abort "échec téléchargement k3s"

  log "k3s: verify checksum"
  if ! verify_checksum_from_sums "$bin_tmp" "$sums_tmp" "k3s"; then
    rm -f "$bin_tmp" "$sums_tmp"
    abort "checksum k3s invalide"
  fi

  log "k3s: install binary"
  run_and_log mv "$bin_tmp" /usr/local/bin/k3s || abort "mv failed"
  run_and_log chmod 0755 /usr/local/bin/k3s || true
  rm -f "$sums_tmp"
  log "k3s ${version} installé dans /usr/local/bin/k3s"

  # minimal systemd unit setup: prefer k3s install script normally, but we keep minimal surface
  log "k3s: create log dir /var/log/k3s and set permissions"
  run_and_log mkdir -p /var/log/k3s || true
  run_and_log chown root:root /var/log/k3s || true
  run_and_log chmod 0700 /var/log/k3s || true

  end_step "install_k3s"
}

main(){
  start_step "main"
  log "Script args: $*"
  log "CONFIG_FILE=$CONFIG_FILE"

  require_root; check_arch; parse_components

  # Terraform
  if contains_component "terraform"; then
    local tfv="${TF_VERSION:-}"
    tfv="${tfv:-${comp_props[terraform.version]:-}}"
    [[ -n "${tfv}" ]] || abort "Terraform demandé mais pas de version fournie (TF_VERSION env or terraform.version in YAML)"
    if ! command -v terraform >/dev/null 2>&1; then
      install_terraform "$tfv"
    else
      log "Terraform déjà présent; saut."
    fi
  fi

  # k3s
  if contains_component "k3s"; then
    local k3sv="${comp_props[k3s.version]:-}"
    [[ -n "${k3sv}" ]] || abort "k3s demandé mais aucune version fournie (k3s.version required in YAML)"
    if ! command -v k3s >/dev/null 2>&1; then
      install_k3s "$k3sv"
    else
      log "k3s déjà présent; saut."
    fi
  fi

  log "Terminé."
  end_step "main"
}

# Script entry point
start_step "script"
log "Script start: $(basename "$0")"
main "$@"
log "Script finished: $(basename "$0")"
end_step "script"