#!/usr/bin/env bash
# Roll the API image for one commit out to k3s. Run ON the control host (by CI through SSM
# Run Command, or by hand):  scripts/k3s/deploy.sh <git sha>
# Secrets are not touched here; sync-secrets.sh owns them. The Helm chart comes from the
# public repo, so no credentials are needed to fetch them. On a failed rollout the
# previous ReplicaSet is restored and the script exits non-zero.
set -euo pipefail

main() {
  local sha="${1:-}"
  if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
    echo "usage: deploy.sh <40-char git sha>" >&2
    return 2
  fi
  local repo_url="${LEETFORCE_REPO_URL:-https://github.com/KarthikSP1911/Leetforce.git}"
  local dir="${LEETFORCE_DEPLOY_DIR:-/opt/leetforce}"
  local image="${LEETFORCE_API_IMAGE:-ghcr.io/karthiksp1911/leetforce-api}"
  local kubectl="${KUBECTL:-k3s kubectl}"
  local helm="${HELM:-helm}"
  export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

  if [ ! -d "$dir/.git" ]; then
    git clone --depth 1 "$repo_url" "$dir"
  fi
  git -C "$dir" fetch --depth 1 origin "$sha"
  git -C "$dir" checkout -qf "$sha"

  # The namespace (Pod Security labels) stays outside the chart: sync-secrets.sh creates it
  # first, and Helm cannot adopt a namespace it did not create.
  echo "deploy: applying $image:$sha"
  $kubectl apply -f "$dir/k8s/namespace.yaml"
  # --wait is off: the rollout check below decides, then helm rollback restores the last release.
  $helm upgrade --install api "$dir/k8s/charts/leetforce-api" -n leetforce \n    --set image.repository="$image" --set image.tag="$sha" --wait=false

  if $kubectl -n leetforce rollout status deploy/api --timeout=180s; then
    echo "deploy: $sha is live"
    return 0
  fi

  echo "deploy: rollout failed, rolling back" >&2
  # The first ever rollout has no previous revision to undo; that is not a second error.
  $helm rollback api -n leetforce --wait=false || true
  $kubectl -n leetforce rollout status deploy/api --timeout=120s || true
  return 1
}

# Everything above is parsed before it runs, so a checkout that replaces this file cannot corrupt it.
main "$@"
exit $?
