#!/usr/bin/env bash
# Recreates the throwaway ghc-e2e Mattermost from nothing, installs the plugin and routes acme to engineering/github.
set -euo pipefail
cd "$(dirname "$0")"
docker compose down -v
docker compose up -d
for i in $(seq 1 60); do curl -sf http://127.0.0.1:18165/api/v4/system/ping >/dev/null && break; sleep 3; done
X="docker exec ghc-e2e-mattermost-1 mmctl --local"
$X user create --email admin@example.com --username admin --password 'Fake-e2e-Pass-123!' --system-admin >/dev/null
$X team create --name engineering --display-name Engineering >/dev/null
$X team users add engineering admin
$X channel create --team engineering --name github --display-name GitHub >/dev/null
$X channel users add engineering:github admin
docker cp "$(ls ../dist/com.scaleupconsulting.githubcards-*.tar.gz | tail -1)" ghc-e2e-mattermost-1:/tmp/ghc.tar.gz
$X plugin add /tmp/ghc.tar.gz
jq -n '{PluginSettings:{Plugins:{"com.scaleupconsulting.githubcards":{webhooksecrets:"acme = fake-e2e-secret-not-real-0001", routes:"acme -> engineering/github"}}}}' > "${TMPDIR:-/tmp}/ghc-patch.json"
docker cp "${TMPDIR:-/tmp}/ghc-patch.json" ghc-e2e-mattermost-1:/tmp/patch.json
$X config patch /tmp/patch.json >/dev/null
$X plugin enable com.scaleupconsulting.githubcards
