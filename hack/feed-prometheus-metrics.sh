#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

# feed-prometheus-metrics.sh drives synthetic metric load into a shoot control-plane
# Prometheus so its TSDB (and thus the prometheus-db PVC) fills up. It is a local
# development / testing helper for exercising PVC autoscaling of the shoot Prometheus.
#
# It deploys the Prometheus community "avalanche" load generator as a Deployment with
# multiple replicas in the shoot control-plane namespace on the seed. Each replica
# exposes a large set of synthetic series at /metrics, fronted by a Service. A
# ServiceMonitor (selected by the shoot Prometheus) makes Prometheus scrape that Service
# at a short interval, so every scrape appends the full series set to the TSDB head.
#
# Total series ingested per scrape is roughly:
#   gauge-metric-count x series-count x replicas
# and that whole set is appended every scrape-interval seconds.
#
# Unlike remote-write, scraping needs no change to the Prometheus resource (no
# remote-write receiver), and scrape success/failure is directly visible via the
# target's "up" metric and last-scrape status.
#
# Example:
#   hack/feed-prometheus-metrics.sh \
#     --namespace shoot--local--foo \
#     --kubeconfig ./example/gardener-local/kind/local/kubeconfig \
#     --gauge-metric-count 1000 --series-count 300 --replicas 3 --scrape-interval 5s

set -o errexit
set -o nounset
set -o pipefail

# ---- defaults -------------------------------------------------------------------------

NAMESPACE=""
KUBECONFIG_FLAG=()
PROMETHEUS_NAME="shoot"
METRICS_PORT="9001"        # port avalanche serves /metrics on (and the Service/target port)
GAUGE_METRIC_COUNT=1000    # number of distinct gauge metric names per replica
SERIES_COUNT=300           # number of series per metric name (distinct label-value combos)
LABEL_COUNT=10             # labels per series (wider series => more bytes on disk)
REPLICAS=3                 # avalanche Deployment replicas (series scale linearly with this)
SCRAPE_INTERVAL="5s"       # how often Prometheus scrapes the generator (shorter => faster fill)
DURATION=10800             # seconds to generate load before stopping (3h, past the first head compaction)
AVALANCHE_IMAGE="quay.io/prometheuscommunity/avalanche:v0.6.0"
APP_NAME="prometheus-load-generator"

# ---- usage ----------------------------------------------------------------------------

usage() {
  cat <<EOF
Usage: $(basename "$0") --namespace <shoot-control-plane-namespace> [options]

Deploys the avalanche load generator as a scrape target and configures the shoot
Prometheus to scrape it, filling the prometheus-db PVC. Total series per scrape ~=
gauge-metric-count x series-count x replicas.

Required:
  --namespace, -n <ns>          Shoot control-plane namespace on the seed (e.g. shoot--local--foo).

Options:
  --kubeconfig <path>           Kubeconfig of the seed cluster (defaults to the current context).
  --gauge-metric-count <n>      Distinct gauge metric names per replica (default: ${GAUGE_METRIC_COUNT}).
  --series-count <n>            Series per metric name (default: ${SERIES_COUNT}).
  --label-count <n>             Labels per series (default: ${LABEL_COUNT}).
  --replicas <n>                avalanche Deployment replicas (default: ${REPLICAS}).
  --scrape-interval <dur>       Prometheus scrape interval, e.g. 5s, 15s (default: ${SCRAPE_INTERVAL}).
  --duration <seconds>          Seconds to generate load before stopping (default: ${DURATION}).
  --image <ref>                 avalanche container image (default: ${AVALANCHE_IMAGE}).
  --help, -h                    Show this help.
EOF
}

# ---- arg parsing ----------------------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace|-n)           NAMESPACE="$2"; shift 2 ;;
    --kubeconfig)             KUBECONFIG_FLAG=(--kubeconfig "$2"); shift 2 ;;
    --gauge-metric-count)     GAUGE_METRIC_COUNT="$2"; shift 2 ;;
    --series-count)           SERIES_COUNT="$2"; shift 2 ;;
    --label-count)            LABEL_COUNT="$2"; shift 2 ;;
    --replicas)               REPLICAS="$2"; shift 2 ;;
    --scrape-interval)        SCRAPE_INTERVAL="$2"; shift 2 ;;
    --duration)               DURATION="$2"; shift 2 ;;
    --image)                  AVALANCHE_IMAGE="$2"; shift 2 ;;
    --help|-h)                usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "${NAMESPACE}" ]]; then
  echo "error: --namespace is required" >&2
  usage
  exit 1
fi

kubectl_cmd() {
  kubectl "${KUBECONFIG_FLAG[@]}" -n "${NAMESPACE}" "$@"
}

# ---- preconditions --------------------------------------------------------------------

if ! command -v kubectl >/dev/null 2>&1; then
  echo "error: kubectl not found in PATH" >&2
  exit 1
fi

if ! kubectl_cmd get prometheus "${PROMETHEUS_NAME}" >/dev/null 2>&1; then
  echo "error: Prometheus resource '${PROMETHEUS_NAME}' not found in namespace '${NAMESPACE}'" >&2
  exit 1
fi

INGRESS_POLICY_NAME="allow-${PROMETHEUS_NAME}-prometheus-to-${APP_NAME}"
EGRESS_POLICY_NAME="allow-${PROMETHEUS_NAME}-prometheus-egress-to-${APP_NAME}"

cleanup() {
  echo
  echo "cleaning up..."
  [[ -n "${LOGS_PID:-}" ]] && kill "${LOGS_PID}" >/dev/null 2>&1 || true
  kubectl_cmd delete servicemonitor "${APP_NAME}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  kubectl_cmd delete deployment "${APP_NAME}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  kubectl_cmd delete service "${APP_NAME}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  kubectl_cmd delete networkpolicy "${INGRESS_POLICY_NAME}" "${EGRESS_POLICY_NAME}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

# ---- network policies -----------------------------------------------------------------

# Every shoot control-plane namespace has a deny-all NetworkPolicy (empty podSelector,
# both Ingress and Egress), so the scrape path must be opened explicitly in both
# directions. Scraping is Prometheus -> generator, so:
#   * the generator pods need Ingress from the Prometheus pods on the metrics port, and
#   * the Prometheus pods need Egress to the generator pods on the metrics port.
# Both are created here directly against the real pod selectors ('app=${APP_NAME}' for the
# generator, 'prometheus=${PROMETHEUS_NAME}' for the Prometheus pods) rather than relying
# on the resource-manager's label-driven auto-generation, which only covers services it
# manages and whose egress label does not survive a gardenlet reconcile of Prometheus.
echo "creating NetworkPolicies for scraping (${PROMETHEUS_NAME} prometheus -> ${APP_NAME}:${METRICS_PORT})"
kubectl_cmd apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: ${INGRESS_POLICY_NAME}
  namespace: ${NAMESPACE}
spec:
  podSelector:
    matchLabels:
      app: ${APP_NAME}
  policyTypes:
  - Ingress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          prometheus: ${PROMETHEUS_NAME}
    ports:
    - protocol: TCP
      port: ${METRICS_PORT}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: ${EGRESS_POLICY_NAME}
  namespace: ${NAMESPACE}
spec:
  podSelector:
    matchLabels:
      prometheus: ${PROMETHEUS_NAME}
  policyTypes:
  - Egress
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: ${APP_NAME}
    ports:
    - protocol: TCP
      port: ${METRICS_PORT}
EOF

# ---- deployment + service -------------------------------------------------------------

echo "deploying avalanche load generator '${APP_NAME}'"
echo "  replicas            : ${REPLICAS}"
echo "  metrics per replica : ${GAUGE_METRIC_COUNT} gauges x ${SERIES_COUNT} series x ${LABEL_COUNT} labels"
echo "  total series        : ~$(( GAUGE_METRIC_COUNT * SERIES_COUNT * REPLICAS )) (scraped every ${SCRAPE_INTERVAL})"
echo "  duration            : ${DURATION}s"
echo

kubectl_cmd apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${APP_NAME}
  namespace: ${NAMESPACE}
  labels:
    app: ${APP_NAME}
spec:
  replicas: ${REPLICAS}
  selector:
    matchLabels:
      app: ${APP_NAME}
  template:
    metadata:
      labels:
        app: ${APP_NAME}
        networking.gardener.cloud/to-dns: allowed
    spec:
      containers:
      - name: avalanche
        image: ${AVALANCHE_IMAGE}
        args:
        - --port=${METRICS_PORT}
        - --gauge-metric-count=${GAUGE_METRIC_COUNT}
        - --series-count=${SERIES_COUNT}
        - --label-count=${LABEL_COUNT}
        # Keep the series set stable so every scrape appends the same large set
        # (no churn needed; scraping, not remote-write, drives the ingest).
        - --series-interval=0
        - --metric-interval=0
        - --value-interval=10
        ports:
        - name: metrics
          containerPort: ${METRICS_PORT}
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
---
apiVersion: v1
kind: Service
metadata:
  name: ${APP_NAME}
  namespace: ${NAMESPACE}
  labels:
    app: ${APP_NAME}
spec:
  selector:
    app: ${APP_NAME}
  ports:
  - name: metrics
    protocol: TCP
    port: ${METRICS_PORT}
    targetPort: metrics
EOF

# ---- scrape config --------------------------------------------------------------------

# ---- service monitor ------------------------------------------------------------------

# The ServiceMonitor must carry the label 'prometheus: ${PROMETHEUS_NAME}' to be selected
# by the shoot Prometheus (its ServiceMonitorSelector matches that label). It selects the
# generator Service and scrapes its 'metrics' endpoint at the configured interval.
echo "creating ServiceMonitor '${APP_NAME}' (scrape interval ${SCRAPE_INTERVAL})"
kubectl_cmd apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: ${APP_NAME}
  namespace: ${NAMESPACE}
  labels:
    prometheus: ${PROMETHEUS_NAME}
spec:
  selector:
    matchLabels:
      app: ${APP_NAME}
  endpoints:
  - port: metrics
    interval: ${SCRAPE_INTERVAL}
EOF

# ---- wait + observe -------------------------------------------------------------------

echo "waiting for the load generator Deployment to become available..."
if ! kubectl_cmd rollout status "deployment/${APP_NAME}" --timeout=180s; then
  echo
  echo "error: Deployment '${APP_NAME}' did not become available within the timeout." >&2
  echo "----- pods -----" >&2
  kubectl_cmd get pods -l "app=${APP_NAME}" -o wide >&2 || true
  echo "----- recent events -----" >&2
  kubectl_cmd describe deployment "${APP_NAME}" 2>&1 | sed -n '/^Events:/,$p' >&2 || true
  exit 1
fi

echo
echo "load generator is running and the ServiceMonitor is in place."
echo "Verify Prometheus is scraping it (target should be 'up') and watch the PVC grow:"
echo "  kubectl ${KUBECONFIG_FLAG[*]} -n ${NAMESPACE} port-forward svc/prometheus-${PROMETHEUS_NAME} 9090:80"
echo "    then query: up{job=\"${APP_NAME}\"}"
echo "                rate(prometheus_tsdb_head_samples_appended_total[1m])"
echo "  watch kubectl ${KUBECONFIG_FLAG[*]} -n ${NAMESPACE} get pvc,persistentvolumeclaimautoscaler"
echo
echo "Running for ${DURATION}s. Streaming generator logs (Ctrl-C to stop and clean up)."
echo

# Follow the generator logs on a best-effort basis. A transient API connection drop makes
# `kubectl logs -f` exit; reconnect in a loop so a blip does not end the stream. This runs
# in the background so the log stream never governs the script's lifetime - the test keeps
# running (and keeps its resources) even while logs are temporarily unavailable.
follow_logs() {
  while true; do
    kubectl_cmd logs -f --since=1s "deployment/${APP_NAME}" 2>/dev/null || true
    # Stop looping once the deployment is gone (cleanup ran / duration elapsed).
    kubectl_cmd get deployment "${APP_NAME}" >/dev/null 2>&1 || break
    sleep 2
  done
}
follow_logs &
LOGS_PID=$!

# The test's lifetime is this foreground sleep: it runs for the requested duration and is
# interruptible by Ctrl-C (which triggers the EXIT/INT/TERM trap -> cleanup). Cleanup is
# done exactly once, by the trap - the duration timer just ends this sleep.
sleep "${DURATION}" || true

kill "${LOGS_PID}" >/dev/null 2>&1 || true
echo "duration elapsed."
