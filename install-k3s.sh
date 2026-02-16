#!/usr/bin/env bash
# install-k3s.sh
# Simple installer pour k3s (server ou agent)
# Usage examples:
#  sudo ./install-k3s.sh --mode server --version v1.27.6+k3s1
#  sudo ./install-k3s.sh --mode agent --url https://1.2.3.4:6443 --token mytoken

set -euo pipefail

usage() {
  cat <<EOF
Usage: $0 [options]

Options:
  -m, --mode [server|agent]   Mode d'installation (default: server)
  -v, --version VERSION       Version k3s (ex: v1.27.6+k3s1)
  -t, --token TOKEN           Token pour joindre un agent au server
  -u, --url URL               URL du serveur k3s (ex: https://1.2.3.4:6443) - requis pour agent
  -n, --node-ip IP            Adresse IP du nœud (optionnel)
  -d, --disable COMPONENTS    Composants à désactiver (ex: traefik,servicelb)
  -y, --yes                   Exécute sans demander confirmation
  -h, --help                  Affiche cette aide
EOF
  exit 1
}

# Defaults
MODE="server"
VERSION=""
TOKEN=""
URL=""
NODE_IP=""
DISABLE=""
ASSUME_YES=false

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    -m|--mode) MODE="$2"; shift 2 ;; 
    -v|--version) VERSION="$2"; shift 2 ;; 
    -t|--token) TOKEN="$2"; shift 2 ;; 
    -u|--url) URL="$2"; shift 2 ;; 
    -n|--node-ip) NODE_IP="$2"; shift 2 ;; 
    -d|--disable) DISABLE="$2"; shift 2 ;; 
    -y|--yes) ASSUME_YES=true; shift ;; 
    -h|--help) usage ;; 
    *) echo "Option inconnue: $1"; usage ;; 
  esac
done

if [[ $EUID -ne 0 ]]; then
  echo "Ce script doit être exécuté en root (sudo)."
  exit 2
fi

if [[ "$MODE" != "server" && "$MODE" != "agent" ]]; then
  echo "Mode invalide: $MODE"
  usage
fi

if [[ "$MODE" == "agent" && -z "$URL" ]]; then
  echo "Pour le mode agent, --url est requis."
  exit 3
fi

echo "Mode: $MODE"
[[ -n "$VERSION" ]] && echo "Version demandée: $VERSION"
[[ -n "$NODE_IP" ]] && echo "Node IP: $NODE_IP"
[[ -n "$DISABLE" ]] && echo "Composants à désactiver: $DISABLE"

if ! $ASSUME_YES; then
  read -p "Continuer ? [y/N] " ans
  case "$ans" in
    [yY][eE][sS]|[yY]) : ;; 
    *) echo "Annulation."; exit 0 ;; 
  esac
fi

# Build env and args for installer
ENV_VARS=()
INSTALL_ARGS=()

if [[ -n "$VERSION" ]]; then
  ENV_VARS+=( "INSTALL_K3S_VERSION=${VERSION}" )
fi

if [[ -n "$DISABLE" ]]; then
  INSTALL_ARGS+=( "--disable=${DISABLE}" )
fi

if [[ -n "$NODE_IP" ]]; then
  INSTALL_ARGS+=( "--node-ip=${NODE_IP}" )
fi

if [[ "$MODE" == "server" ]]; then
  INSTALL_ARGS+=( "server" "--write-kubeconfig-mode" "0644" )
  echo "Installation k3s server..."
  # Run installer
  (
    for v in "${ENV_VARS[@]}"; do export "$v"; done
    curl -sfL https://get.k3s.io | sh -s - "${INSTALL_ARGS[@]}"
  )
  echo "k3s server installé. Vérifie le statut avec: sudo systemctl status k3s"
  echo "Kubeconfig: /etc/rancher/k3s/k3s.yaml"
else
  # agent
  if [[ -z "$TOKEN" ]]; then
    echo "Aucun token fourni. Si le server demande un token, fournis --token."
  fi
  echo "Installation k3s agent..."
  (
    # set env for agent join
    [[ -n "$URL" ]] && export K3S_URL="$URL"
    [[ -n "$TOKEN" ]] && export K3S_TOKEN="$TOKEN"
    for v in "${ENV_VARS[@]}"; do export "$v"; done
    curl -sfL https://get.k3s.io | sh -s - agent "${INSTALL_ARGS[@]}"
  )
  echo "k3s agent installé. Vérifie le statut avec: sudo systemctl status k3s-agent"
fi

echo "Installation terminée."