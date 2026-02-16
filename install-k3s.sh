#!/usr/bin/env bash
# Installe k3s et/ou Terraform uniquement si listés dans install-config.yaml
# Format YAML attendu (options placées sous chaque composant) :
# components:
#   - terraform:
#       version: "1.5.7"
#   - k3s:
#       disable: "traefik,servicelb"
#       extra_args: "--node-ip=10.0.0.5"
#
# Cible: Linux x86_64 (amd64), non-interactif, utilise apt-get si nécessaire pour dépendances.
set -euo pipefail

CONFIG_FILE="[0m"
TF_VERSION_ENV="[0m"

err() { echo "ERREUR: $*" >&2; }

# root check
if [[ $EUID -ne 0 ]]; then
  err "exécuter en root (sudo)."
  exit 2
fi

# arch check
ARCH_RAW="$(uname -m)"
if [[ "$ARCH_RAW" != "x86_64" && "$ARCH_RAW" != "amd64" ]]; then
  err "script conçu pour linux x86_64 (amd64). Architecture détectée: $ARCH_RAW"
  exit 3
fi
ARCH="amd64"

# if no config file -> no-op
if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "Fichier de configuration $CONFIG_FILE introuvable. Aucune action effectuée."
  exit 0
fi

# --- Parse components list with nested mappings ---
# Expect:
# components:
#   - terraform:
#       version: "1.5.7"
#   - k3s:
#       disable: "traefik"
#
# The awk below emits lines:
# COMPONENT:<name>
#   key: value
parse_output=$(awk '
  BEGIN { in=0 }
  /^[0mcomponents[0m:[0m/ { in=1; next }
  in {
    # line like: - name:
    if (match($0, /^[0m-[0m([a-zA-Z0-9_-]+)[0m:[0m/, m)) {
      print "COMPONENT:" m[1]
      next
    }
    # nested lines with at least two spaces indentation, like: "    key: value"
    if (match($0, /^[0m [0m{2,}([a-zA-Z0-9._-]+)[0m:[0m(.*)$/, m)) {
      k=m[1]; v=m[2]; gsub(/^[ 	]+|[ 	]+$/, "", v)
      print "  " k ":" v
      next
    }
    # stop processing components at next top-level key
    if (in && match($0, /^[^[:space:]]/)) { exit }
  }
' "$CONFIG_FILE")

# Build associative map comp_props["component.key"]=value
declare -A comp_props
declare -a components_list
current_comp=""
while IFS= read -r line; do
  [[ -z "${line:-}" ]] && continue
  if [[ "$line" == COMPONENT:* ]]; then
    current_comp="${line#COMPONENT:}"
    components_list+=("$current_comp")
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]+([a-zA-Z0-9._-]+):(.*)$ ]]; then
    key="${BASH_REMATCH[1]}"
    val="${BASH_REMATCH[2]}"
    # trim leading/trailing spaces
    val="${val#"${val%%[![:space:]]*}"}"
    val="${val%"${val##*[![:space:]]}"}"
    # remove surrounding quotes if present
    if [[ "$val" =~ ^"(.*)"$ ]]; then val="${BASH_REMATCH[1]}"; fi
    if [[ "$val" =~ ^"(.*)"$ ]]; then val="${BASH_REMATCH[1]}"; fi
    comp_props["${current_comp}.${key}"]="$val"
  fi
done <<< "$parse_output"

# Helper to check presence
contains_component() {
  local target="$1"
  for c in "${components_list[@]}"; do
    if [[ "$c" == "$target" ]]; then
      return 0
    fi
  done
  return 1
}

# Decide installs
INSTALL_K3S=false
INSTALL_TF=false
if contains_component "k3s"; then INSTALL_K3S=true; fi
if contains_component "terraform"; then INSTALL_TF=true; fi

if [[ "$INSTALL_K3S" != true && "$INSTALL_TF" != true ]]; then
  echo "Aucune des options 'k3s' ou 'terraform' présente dans components. Aucune action effectuée."
  exit 0
fi

# Utilities
is_k3s_installed() {
  command -v k3s >/dev/null 2>&1 || [[ -f /usr/local/bin/k3s || -f /usr/bin/k3s ]] || \
    ( command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet k3s )
}

is_terraform_installed() {
  command -v terraform >/dev/null 2>&1
}

ensure_packages() {
  local pkgs=("$@")
  local miss=()
  for p in "${pkgs[@]}"; do
    if ! command -v "$p" >/dev/null 2>&1; then
      miss+=("$p")
    fi
  done
  if [[ ${#miss[@]} -gt 0 ]]; then
    if command -v apt-get >/dev/null 2>&1; then
      apt-get update -y
      apt-get install -y "${miss[@]}"
    else
      err "Paquets manquants: ${miss[*]}. Aucune apt-get trouvée; installe-les manuellement."
      exit 4
    fi
  fi
}

# k3s default secure flags (we keep traefik/servicelb/local-storage by default)
K3S_EXEC_FLAGS_BASE="server --write-kubeconfig-mode 0640 \
--kube-apiserver-arg=anonymous-auth=false \
--kube-apiserver-arg=audit-log-path=/var/log/k3s/audit.log \
--kube-apiserver-arg=audit-log-maxage=30 \
--kube-apiserver-arg=audit-log-maxbackup=10 \
--kube-apiserver-arg=audit-log-maxsize=100 \
--kubelet-arg=anonymous-auth=false"

prepare_audit_dir() {
  local adir="/var/log/k3s"
  if [[ ! -d "$adir" ]]; then
    mkdir -p "$adir"
  fi
  chmod 700 "$adir"
  chown root:root "$adir"
}

# --- Install k3s if listed ---
if [[ "$INSTALL_K3S" == true ]]; then
  if is_k3s_installed; then
    echo "k3s déjà installé — installation de k3s sautée."
  else
    echo "=> Préparation: création répertoire d'audit pour k3s..."
    prepare_audit_dir

    # build flags: start from base, append disable/extra_args if provided in YAML
    K3S_EXEC_FLAGS="$K3S_EXEC_FLAGS_BASE"
    if [[ -n "${comp_props["k3s.disable"]:-}" && "${comp_props["k3s.disable"]}" != "none" ]]; then
      K3S_EXEC_FLAGS+=" --disable=${comp_props["k3s.disable"]}"
    fi
    if [[ -n "${comp_props["k3s.extra_args"]:-}" ]]; then
      K3S_EXEC_FLAGS+=" ${comp_props["k3s.extra_args"]}"
    fi

    echo "=> Installation k3s (server) via get.k3s.io avec flags:"
    echo "   ${K3S_EXEC_FLAGS}"
    export INSTALL_K3S_EXEC="${K3S_EXEC_FLAGS}"
    curl -fsSL https://get.k3s.io | sh -s - server
    echo "=> k3s installé."
  fi

  # post-install: restrict kubeconfig and allow non-root use via 'k3s' group
  KCFG="/etc/rancher/k3s/k3s.yaml"
  if [[ -f "$KCFG" ]]; then
    echo "=> Appliquer permissions sécurisées au kubeconfig..."
    if ! getent group k3s >/dev/null 2>&1; then
      groupadd --system k3s || true
    fi
    chown root:k3s "$KCFG" || true
    chmod 0640 "$KCFG" || true

    if [[ -n "${SUDO_USER:-}" && "$SUDO_USER" != "root" ]]; then
      usermod -aG k3s "$SUDO_USER" || true
      user_home="$(getent passwd "$SUDO_USER" | cut -d: -f6)"
      if [[ -n "$user_home" ]]; then
        mkdir -p "$user_home/.kube"
        cp -f "$KCFG" "$user_home/.kube/config"
        chown "$SUDO_USER":k3s "$user_home/.kube/config"
        chmod 0640 "$user_home/.kube/config"
      fi
    fi
  else
    echo "ATTENTION: kubeconfig non trouvé à $KCFG — vérifie l'installation de k3s."
  fi
fi

# --- Install Terraform if listed ---
if [[ "$INSTALL_TF" == true ]]; then
  if is_terraform_installed; then
    echo "Terraform déjà installé — installation de Terraform sautée."
  else
    ensure_packages curl unzip

    # TF version resolution order:
    # 1) TF_VERSION env var
    # 2) terraform.version in component mapping
    # 3) autodetect latest
    TF_VERSION="${TF_VERSION_ENV:-}"
    if [[ -z "$TF_VERSION" ]]; then
      TF_VERSION="${comp_props["terraform.version"]:-}"
    fi
    if [[ -z "$TF_VERSION" ]]; then
      TF_VERSION="
      $(curl -fsS https://releases.hashicorp.com/terraform/ | grep -Eo '/terraform/[0-9]+[0m.[0m[0-9]+[0m.[0m[0-9]+' | head -1 | sed 's#/terraform/##' || true)"
    fi
    if [[ -z "$TF_VERSION" ]]; then
      err "Impossible de déterminer la version Terraform. Fournis TF_VERSION env ou terraform.version in YAML."
      exit 5
    fi

    echo "=> Installation Terraform ${TF_VERSION} pour linux_${ARCH}..."
    TF_ZIP="/tmp/terraform_${TF_VERSION}_linux_${ARCH}.zip"
    TF_URL="https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_${ARCH}.zip"

    curl -fsSL -o "${TF_ZIP}" "${TF_URL}"
    unzip -o "${TF_ZIP}" -d /usr/local/bin
    chmod +x /usr/local/bin/terraform
    rm -f "${TF_ZIP}"
    echo "=> Terraform ${TF_VERSION} installé dans /usr/local/bin/terraform"
  fi
fi

echo "Terminé."
if [[ "$INSTALL_K3S" == true ]]; then
  echo "kubeconfig: /etc/rancher/k3s/k3s.yaml (root:k3s, 0640)"
fi
