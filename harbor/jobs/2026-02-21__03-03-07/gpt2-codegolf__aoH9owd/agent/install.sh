#!/bin/bash
set -euo pipefail

# Install system dependencies (git is required for go install).
if command -v apt-get &>/dev/null; then
    apt-get update -qq && apt-get install -y -qq curl git ca-certificates >/dev/null 2>&1
elif command -v apk &>/dev/null; then
    apk add --no-cache curl git ca-certificates >/dev/null 2>&1
fi

# Install Go 1.25.6+ (required by go.mod).
GO_VERSION="1.25.6"
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf /tmp/go.tar.gz
rm /tmp/go.tar.gz

export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
export GOPATH="$HOME/go"

# Verify Go installation.
go version

# Install gollem CLI.

go install github.com/fugue-labs/gollem/cmd/gollem@latest


# Verify gollem installation.
"$GOPATH/bin/gollem" --help >/dev/null 2>&1 || "$HOME/go/bin/gollem" --help >/dev/null 2>&1
echo "gollem installed successfully"