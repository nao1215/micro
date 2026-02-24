#!/bin/bash
set -euo pipefail

echo "=== Docker Engine インストール (Ubuntu) ==="

# 前提パッケージのインストール
sudo apt-get update
sudo apt-get install -y ca-certificates curl

# Docker公式GPGキーの追加
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

# リポジトリの追加
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# Docker Engineのインストール
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# 現在のユーザーをdockerグループに追加
sudo usermod -aG docker "$USER"

echo ""
echo "=== インストール完了 ==="
echo "グループ変更を反映するため、以下を実行してください:"
echo "  newgrp docker"
echo ""
echo "その後、以下でE2Eテストを実行できます:"
echo "  make docker-up && make e2e"
