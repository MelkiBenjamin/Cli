#!/usr/bin/env bash
# Minimal, non-interactive, hardened k3s + Terraform installer (linux x86_64/amd64)
# - N'installe k3s et terraform que s'ils ne sont pas déjà présents.
# - Garde traefik, servicelb et local-storage (par défaut).
# - Applique des bonnes pratiques de durcissement basiques :
#   * désactive l'accès anonyme à l'API
#   * active l'audit logging
#   * restreint l'accès anonyme au kubelet
#   * kubeconfig en 0640 et accès via groupe 'k3s' pour un utilisateur non-root
# - Pas d'utilisation de yum; apt-get uniquement si nécessaire pour dépendances.
#
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

# Vérifier arch (amd64 requis)
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
  local pkgs=( "$@" )
  local miss=()
  for p in "${pkgs[@]}"; do
    if ! command -v "$p" >/dev/null 2>&1; then
      miss+=( "$p" )
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

# Flags k3s sécurisés (traefik, servicelb, local-storage laissés par défaut)
# Bonnes pratiques minimales : disable anonymous auth, audit logs, kubelet anonymous off
K3S_EXEC_FLAGS="server --write-kubeconfig-mode 0640 \
--kube-apiserver-arg=anonymous-auth=false \
--kube-apiserver-arg=audit-log-path=/var/log/k3s/audit.log \
--kube-apiserver-arg=audit-log-maxage=30 \
--kube-apiserver-arg=audit-log-maxbackup=10 \
--kube-apiserver-arg=audit-log-maxsize=100 \
--kubelet-arg=anonymous-auth=false"

# Préparer répertoire d'audit et permissions
prepare_audit_dir() {
  local adir="/var/log/k3s"
  if [[ ! -d "$adir" ]]; then
    mkdir -p "$adir"
  fi
  chmod 700 "$adir"
  chown root:root "$adir"
}

# Installer k3s si absent
if is_k3s_installed; then
  echo "k3s déjà installé — installation de k3s sautée."
else
  echo "=> Préparation du durcissement: création répertoire audit..."
  prepare_audit_dir

  echo "=> Installation k3s (server) via get.k3s.io avec flags sécurisés..."
  # Exporter pour l'installateur officiel (ou bien le passer en argument)
  export INSTALL_K3S_EXEC="${K3S_EXEC_FLAGS}"
  curl -fsSL https://get.k3s.io | sh -s - server
  echo "=> k3s installé."
fi

# Post-install: restreindre kubeconfig et permettre usage non-root via groupe 'k3s'
KCFG="/etc/rancher/k3s/k3s.yaml"
if [[ -f "$KCFG" ]]; then
  echo "=> Appliquer permissions sécurisées au kubeconfig..."
  # créer groupe système 'k3s' si nécessaire
  if ! getent group k3s >/dev/null 2>&1; then
    groupadd --system k3s || true
  fi
  chown root:k3s "$KCFG" || true
  chmod 0640 "$KCFG" || true
  echo "Kubeconfig: $KCFG (root:k3s, mode 0640)"

  # Si lancé via sudo, ajouter SUDO_USER au groupe k3s et copier kubeconfig dans son home
  if [[ -n "${SUDO_USER:-}" && "$SUDO_USER" != "root" ]]; then
    echo "=> Ajout de ${SUDO_USER} au groupe k3s pour accès kubeconfig..."
    usermod -aG k3s "$SUDO_USER" || true
    user_home="$(getent passwd "$SUDO_USER" | cut -d: -f6)"
    if [[ -n "$user_home" ]]; then
      mkdir -p "$user_home/.kube"
      cp -f "$KCFG" "$user_home/.kube/config"
      chown "$SUDO_USER":k3s "$user_home/.kube/config"
      chmod 0640 "$user_home/.kube/config"
      echo "Kubeconfig copié vers $user_home/.kube/config (owner ${SUDO_USER}:k3s, 0640)"
    fi
  fi
else
  echo "ATTENTION: kubeconfig non trouvé à $KCFG — vérifie l'installation de k3s."
fi

# Recommendations de post-configuration (imprimées pour l'admin)
echo
echo "Recommandations post-install (à appliquer manuellement si nécessaire) :"
echo "- Restreindre l'accès réseau aux ports k3s (ex: autoriser 6443 uniquement depuis IPs de confiance)."
echo "- Activer et configurer un pare-feu (ufw/iptables) pour n'exposer que les ports nécessaires."
echo "- Envisager l'utilisation d'un store externe pour etcd/HA plutôt que la base embarquée si production."
echo "- Mettre en place admission controllers (ex: OPA Gatekeeper) et appliquer des PodSecurityPolicies / Pod Security Standards."
echo "- Configurer une rotation des certificats et un mécanisme d'audit centralisé pour les logs d'audit."
echo

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
echo "kubeconfig: /etc/rancher/k3s/k3s.yaml (root:k3s, 0640). Si tu veux utiliser kubectl en non-root, connecte-toi avec l'utilisateur ajouté au groupe 'k3s' ou copie ~/.kube/config depuis /etc/rancher/k3s/k3s.yaml."