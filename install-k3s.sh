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
    /^\\s*components\\s*:/ { in=1; next }
    in {
      if (match($0,/^\\s*-\\s*([a-zA-Z0-9_-]+)\\s*:/,m)) { print "COMP:" m[1]; next }
      if (match($0,/^\\s{2,}([a-zA-Z0-9._-]+)\\s*:\s*(.*)$/,m)) { k=m[1]; v=m[2]; gsub(/^[ 	]+|[ 	]+$/,"",v); print "  " k ":" v; next }
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
      v="${v#"${v%%[![:space:]]*}"}"
      v="${v%"${v##*[![:space:]]}]}")
      if [[ "${v:0:1}" == '"' && "${v: -1}" == '"' ]]; then v="${v:1:-1}"; fi
      if [[ "${v:0:1}" == "'" && "${v: -1}" == "'" ]]; then v="${v:1:-1}"; fi
      comp_props["${current}.${k}"]="$v"
    fi
  done <<< "$parse"
}

contains_component(){ local want="$1"; for c in "${components_list[@]:-}"; do [[ "$c" == "$want" ]] && return 0; done; return 1 }

sha256_of_file(){ sha256sum "$1" | awk '{print $1}'; }

verify_checksum_from_sums() {
  local file="$1" sums="$2" needle="$3"
  [[ -f "$sums" ]] || return 1
  local expected actual
  expected=$(grep -E "[[:space:]]${needle}$" "$sums" | awk '{print $1}' | head -1) || true
  actual=$(sha256_of_file "$file") || return 1
  [[ "$actual" == "$expected" ]]
}

download(){ local url="$1" dest="$2"; curl "${CURL_OPTS[@]}" -f -o "$dest" "$url"; }

install_terraform(){
  local version="$1" arch=amd64 fname="terraform_${version}_linux_${arch}.zip"
  local base="https://releases.hashicorp.com/terraform/${version}"
  local zip="/tmp/${fname}" sums="/tmp/terraform_${version}_SHA256SUMS"
  download "${base}/${fname}" "$zip" || abort "échec téléchargement Terraform"
  download "${base}/terraform_${version}_SHA256SUMS" "$sums" || abort "échec téléchargement des checksums Terraform"
  verify_checksum_from_sums "$zip" "$sums" "$fname" || abort "checksum Terraform invalide"
  unzip -o "$zip" -d /usr/local/bin || abort "unzip échec"
  chmod 0755 /usr/local/bin/terraform || true
  rm -f "$zip" "$sums"
}

install_k3s(){
  local version="$1"
  local tag="$version"
  local base="https://github.com/k3s-io/k3s/releases/download/${tag}"
  local bin_tmp="/tmp/k3s-${tag}-amd64" sums_tmp="/tmp/sha256-${tag}.txt"
  local sums_urls=("${base}/sha256sum-amd64.txt" "${base}/sha256sums.txt" "${base}/sha256sum.txt")
  local sums_url found_sums
  for sums_url in "${sums_urls[@]}"; do
    if curl "${CURL_OPTS[@]}" -f -o "$sums_tmp" "$sums_url"; then found_sums=1; break; fi
  done
  [[ -n "${found_sums}" ]] || abort "Impossible de récupérer le fichier de checksums k3s"
  download "${base}/k3s" "$bin_tmp" || abort "échec téléchargement k3s"
  verify_checksum_from_sums "$bin_tmp" "$sums_tmp" "k3s" || { rm -f "$bin_tmp" "$sums_tmp"; abort "checksum k3s invalide"; }
  mv "$bin_tmp" /usr/local/bin/k3s || abort "mv failed"
  chmod 0755 /usr/local/bin/k3s || true
  rm -f "$sums_tmp"
  mkdir -p /var/log/k3s || true
  chown root:root /var/log/k3s || true
  chmod 0700 /var/log/k3s || true
}

main(){
  require_root; check_arch; parse_components
  if contains_component "terraform"; then
    local tfv="${TF_VERSION:-}"
    tfv="${tfv:-${comp_props[terraform.version]:-}}"
    [[ -n "${tfv}" ]] || abort "Terraform demandé mais pas de version fournie"
    if ! command -v terraform >/dev/null 2>&1; then install_terraform "$tfv"; fi
  fi
  if contains_component "k3s"; then
    local k3sv="${comp_props[k3s.version]:-}"
    [[ -n "${k3sv}" ]] || abort "k3s demandé mais aucune version fournie"
    if ! command -v k3s >/dev/null 2>&1; then install_k3s "$k3sv"; fi
  fi
}

main "$@"