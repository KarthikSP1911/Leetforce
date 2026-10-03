#!/usr/bin/env bash
# Roll the API image for one commit out to k3s. Run ON the control host (by CI through SSM
# Run Command, or by hand):  scripts/k3s/deploy.sh <git sha>
# Secrets are not touched here; sync-secrets.sh owns them. The manifests come from the
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

  if [ ! -d "$dir/.git" ]; then
    git clone --depth 1 "$repo_url" "$dir"
  fi
  git -C "$dir" fetch --depth 1 origin "$sha"
  git -C "$dir" checkout -qf "$sha"

  # Kustomize runs on a temp copy so the checked-out tree (and git) never changes.
  local work
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT
  cp -r "$dir/k8s/base/." "$work/"
  sed -i "s|^\( *newTag:\).*|\1 $sha|" "$work/kustomization.yaml"
  if ! grep -q "newTag: $sha\$" "$work/kustomization.yaml"; then
    echo "deploy: could not set the image tag in kustomization.yaml" >&2
    return 1
  fi

  echo "deploy: applying $image:$sha"
  $kubectl apply -k "$work"

  if $kubectl -n leetforce rollout status deploy/api --timeout=180s; then
    echo "deploy: $sha is live"
    return 0
  fi

  echo "deploy: rollout failed, rolling back" >&2
  # The first ever rollout has no previous revision to undo; that is not a second error.
  $kubectl -n leetforce rollout undo deploy/api || true
  $kubectl -n leetforce rollout status deploy/api --timeout=120s || true
  return 1
}

# Everything above is parsed before it runs, so a checkout that replaces this file cannot corrupt it.
main "$@"
exit $?
