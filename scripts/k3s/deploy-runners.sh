#!/usr/bin/env bash
# Install KEDA (once) and roll the runner chart for one commit onto k3s (Phase 17, ADR 0029).
# WRITTEN, NEVER RUN. Run ON the control host, by hand:  scripts/k3s/deploy-runners.sh <git sha>
# It is deliberately NOT part of the CI deploy job: the first rollout must be watched.
#
# Before the first run (all of these are the owner's steps, none were done in Phase 17):
#   1. infra/aws applied with runner_node_count >= 1, and the nodes show as Ready:
#        k3s kubectl get nodes -l leetforce.dev/pool=runner
#   2. the join token and the runner settings are in SSM (push-agent-token.sh, push-ssm.sh)
#   3. the runner image for <sha> exists in GHCR (the deploy workflow's runner-image job)
#
# KEDA: installed from the official Helm repository at a pinned chart version. The chart needs
# the CRDs, so the first install must finish before the runner chart (its ScaledObject) applies.
# Pods that do not fit on the fixed nodes stay Pending; this script does not add nodes.
set -euo pipefail

# Newest release found while writing (UNVERIFIED); Helm chart numbers follow KEDA releases.
KEDA_CHART_VERSION="${KEDA_CHART_VERSION:-2.20.2}"

main() {
  local sha="${1:-}"
  if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
    echo "usage: deploy-runners.sh <40-char git sha>" >&2
    return 2
  fi
  local repo_url="${LEETFORCE_REPO_URL:-https://github.com/KarthikSP1911/Leetforce.git}"
  local dir="${LEETFORCE_DEPLOY_DIR:-/opt/leetforce}"
  local image="${LEETFORCE_RUNNER_IMAGE:-ghcr.io/karthiksp1911/leetforce-runner}"
  local kubectl="${KUBECTL:-k3s kubectl}"
  local helm="${HELM:-helm}"
  export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

  if [ ! -d "$dir/.git" ]; then
    git clone --depth 1 "$repo_url" "$dir"
  fi
  git -C "$dir" fetch --depth 1 origin "$sha"
  git -C "$dir" checkout -qf "$sha"

  # 1. KEDA operator and CRDs.
  if ! $kubectl get crd scaledobjects.keda.sh >/dev/null 2>&1; then
    echo "deploy-runners: installing KEDA chart $KEDA_CHART_VERSION"
    $helm repo add kedacore https://kedacore.github.io/charts >/dev/null
    $helm repo update kedacore >/dev/null
    $helm upgrade --install keda kedacore/keda --version "$KEDA_CHART_VERSION" \
      --namespace keda --create-namespace --wait --timeout 5m
  else
    echo "deploy-runners: KEDA already installed (scaledobjects.keda.sh exists); not touching it"
  fi

  # 2. Namespace (Pod Security labels), Secrets from SSM.
  $kubectl apply -f "$dir/k8s/runners-namespace.yaml"
  bash "$dir/scripts/k3s/sync-runner-secrets.sh"

  # 3. The runner chart. --wait is off: the rollout check below decides, then helm rollback
  #    restores the last release.
  echo "deploy-runners: applying $image:$sha"
  $helm upgrade --install runner "$dir/k8s/charts/leetforce-runner" --namespace leetforce-runners \
    --set image.repository="$image" --set image.tag="$sha" --wait=false

  if $kubectl -n leetforce-runners rollout status deploy/runner --timeout=300s; then
    echo "deploy-runners: $sha is live"
    $kubectl -n leetforce-runners get scaledobject,hpa,pods -o wide
    return 0
  fi

  echo "deploy-runners: rollout failed, rolling back" >&2
  $kubectl -n leetforce-runners get pods -o wide >&2 || true
  $kubectl -n leetforce-runners describe pods -l app.kubernetes.io/name=runner >&2 || true
  # The first ever rollout has no previous revision to undo; that is not a second error.
  $helm rollback runner -n leetforce-runners --wait=false || true
  return 1
}

# Everything above is parsed before it runs, so a checkout that replaces this file cannot corrupt it.
main "$@"
exit $?
