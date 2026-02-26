#!/usr/bin/env bash
set -euo pipefail

# Bootstrap a dedicated x86_64 Linux host for Harbor + Terminal-Bench 2.0.
# Tuned for official leaderboard submission runs (reproducible + low-latency).
#
# Usage:
#   ./bootstrap-cloud-runner.sh [repo_dir]
#
# Example:
#   ./bootstrap-cloud-runner.sh /opt/gollem

REPO_DIR="${1:-$HOME/ws/gollem}"
GO_VERSION="${GO_VERSION:-1.25.6}"

CACHE_ROOT="${CACHE_ROOT:-/var/cache/gollem}"
UV_CACHE_DIR="${UV_CACHE_DIR:-$CACHE_ROOT/uv}"
GOCACHE="${GOCACHE:-$CACHE_ROOT/go-build}"
GOMODCACHE="${GOMODCACHE:-$CACHE_ROOT/go-mod}"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "This script targets Linux hosts."
  exit 1
fi

if [[ "$(uname -m)" != "x86_64" ]]; then
  echo "This script expects an x86_64 host (no qemu emulation)."
  exit 1
fi

SUDO=""
if [[ "${EUID}" -ne 0 ]]; then
  SUDO="sudo"
fi

if ! command -v apt-get >/dev/null 2>&1; then
  echo "This script currently supports Debian/Ubuntu hosts (apt-get)."
  exit 1
fi

echo "[1/7] Installing system packages"
$SUDO apt-get update -qq
$SUDO DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
  ca-certificates curl git jq unzip build-essential \
  python3 python3-venv python3-pip docker.io

$SUDO systemctl enable --now docker
$SUDO usermod -aG docker "$USER" || true

echo "[2/7] Installing Go ${GO_VERSION}"
NEED_GO_INSTALL=1
if command -v go >/dev/null 2>&1; then
  if go version | grep -q "go${GO_VERSION}"; then
    NEED_GO_INSTALL=0
  fi
fi

if [[ "${NEED_GO_INSTALL}" -eq 1 ]]; then
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
  $SUDO rm -rf /usr/local/go
  $SUDO tar -C /usr/local -xzf /tmp/go.tar.gz
  rm -f /tmp/go.tar.gz
fi

export PATH="/usr/local/go/bin:$HOME/.local/bin:$PATH"

echo "[3/7] Installing uv"
if ! command -v uv >/dev/null 2>&1; then
  curl -LsSf https://astral.sh/uv/install.sh | sh
fi
export PATH="$HOME/.local/bin:$PATH"

echo "[4/7] Preparing repository at ${REPO_DIR}"
if [[ ! -d "${REPO_DIR}/.git" ]]; then
  mkdir -p "$(dirname "${REPO_DIR}")"
  git clone https://github.com/fugue-labs/gollem.git "${REPO_DIR}"
fi

echo "[5/7] Creating persistent caches"
$SUDO mkdir -p "${UV_CACHE_DIR}" "${GOCACHE}" "${GOMODCACHE}"
$SUDO chown -R "$USER":"$USER" "${CACHE_ROOT}"

echo "[6/7] Installing Harbor deps + building gollem linux binary"
cd "${REPO_DIR}/harbor"
UV_CACHE_DIR="${UV_CACHE_DIR}" uv sync

cd "${REPO_DIR}"
UV_CACHE_DIR="${UV_CACHE_DIR}" \
GOCACHE="${GOCACHE}" \
GOMODCACHE="${GOMODCACHE}" \
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
go build -o harbor/gollem-linux-amd64 ./cmd/gollem/

echo "[7/7] Writing Harbor env profile"
mkdir -p "$HOME/.config"
ENV_FILE="$HOME/.config/gollem-harbor.env"
cat > "${ENV_FILE}" <<EOF
export UV_CACHE_DIR="${UV_CACHE_DIR}"
export GOCACHE="${GOCACHE}"
export GOMODCACHE="${GOMODCACHE}"
export GOLLEM_MODEL_REQUEST_TIMEOUT_SEC=360
export GOLLEM_TEAM_MODE=off
export GOLLEM_DISABLE_RUNTIME_DEP_INSTALL=1
export GOLLEM_TASK_TIMEOUT_BUFFER_SEC=0
export GOLLEM_SETUP_INSTALL_LSP=1
export GOLLEM_TBENCH_COMPETITION_PROMPT=1
export OPENAI_PROMPT_CACHE_KEY=tbench2-gollem
export OPENAI_PROMPT_CACHE_RETENTION=in_memory
export OPENAI_SERVICE_TIER=priority
export TBENCH_ATTEMPTS=5
export GOLLEM_SETUP_PYTHON_PACKAGES="pytest numpy scipy pandas statsmodels scikit-learn requests pyyaml matplotlib pillow sympy"
export TBENCH_AGENT_URL="https://github.com/fugue-labs/gollem"
export TBENCH_AGENT_DISPLAY_NAME="gollem"
export TBENCH_AGENT_ORG_DISPLAY_NAME="Fugue Labs"
EOF

echo
echo "Bootstrap complete."
echo
echo "Next steps:"
echo "1) Start a new shell or run: newgrp docker"
echo "2) Load env: source ${ENV_FILE}"
echo "3) Add provider creds to ~/.envrc (OPENAI_API_KEY, etc.)"
echo "4) Run official leaderboard eval:"
echo "   cd ${REPO_DIR}/harbor"
echo "   ./run-official-leaderboard.sh openai/gpt-5.3-codex 1"
