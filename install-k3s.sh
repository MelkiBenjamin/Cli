#!/usr/bin/env bash
# Minimal, non-interactive: installe k3s (server) + Terraform (linux x86_64/amd64)
# Comportement: n'installe k3s ou terraform que s'ils ne sont pas déjà présents.
# Usage:
#   sudo TF_VERSION=1.5.7 ./install-k3s.sh
#   sudo ./install-k3s.sh
set -euo pipefail

TF_VERSION="${TF_VERSION:-}"

# Vérifier exécution en root
if [[ $EUID -ne 0 ]]; then
  echo "ERREUR: exécuter en root (sudo)." >&2
  exit 2
fi

# Vérifier architecture (amd64 requis)
ARCH_RAW="$(uname -m)"
if [[ "$ARCH_RAW" != "x86_64" && "$ARCH_RAW" != "amd64" ]]; then
  echo "ERREUR: script conçu pour linux x86_64 (amd64). Architecture détectée: $ARCH_RAW" >&2
  exit 3
fi
ARCH="amd64"

# Détecteurs
is_k3s_installed() {
  if command -v k3s >/dev/null 2>&1; then
    return 0
  fi
  if [[ -f /usr/local/bin/k3s || -f /usr/bin/k3s ]]; then
    return 0
  fi
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet k3s; then
    return 0
  fi
  if [[ -f /etc/rancher/k3s/k3s.yaml ]]; then
    return 0
  fi
  return 1
}

is_terraform_installed() {
  command -v terraform >/dev/null 2>&1
}

# Installer paquets requis via apt-get uniquement (sinon exit)
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
      echo "ERREUR: paquets manquants: ${miss[*]}. Aucune apt-get trouvée; installe-les manuellement." >&2
      exit 4
    fi
  fi
}

# Installer k3s si absent
if is_k3s_installed; then
  echo "k3s déjà installé — installation de k3s sautée."
else
  echo "=> Installation k3s (server) via get.k3s.io ..."
  curl -fsSL https://get.k3s.io | sh -s - server --write-kubeconfig-mode 0644
  echo "=> k3s installé."
fi

# Installer Terraform si absent
if is_terraform_installed; then
  echo "Terraform déjà installé — installation de Terraform sautée."
else
  ensure_packages curl unzip

  if [[ -z "$TF_VERSION" ]]; then
    TF_VERSION="$(curl -fsS https://releases.hashicorp.com/terraform/ | grep -oP '/terraform/\K[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)"
    if [[ -z "$TF_VERSION" ]]; then
      echo "ERREUR: impossible de déterminer la version Terraform automatiquement. Fournis TF_VERSION=... en variable d'environnement." >&2
      exit 5
    fi
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

echo "Terminé."
echo "Si k3s server est installé, kubeconfig: /etc/rancher/k3s/k3s.yaml"