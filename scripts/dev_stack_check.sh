#!/usr/bin/env sh
set -eu

compose_file="${ATLAS_COMPOSE_FILE:-dev/docker-compose.yml}"
api_port="${ATLAS_API_PORT:-8080}"
web_port="${ATLAS_WEB_PORT:-5173}"
dev_issuer_port="${ATLAS_DEV_ISSUER_PORT:-18090}"
timeout_seconds="${ATLAS_DEV_STACK_CHECK_TIMEOUT_SECONDS:-180}"

api_base="http://127.0.0.1:${api_port}"
web_base="http://127.0.0.1:${web_port}"
dev_issuer_base="http://127.0.0.1:${dev_issuer_port}"

compose() {
    docker compose -f "$compose_file" --profile stack "$@"
}

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "required command not found: $1" >&2
        exit 127
    fi
}

print_failure_context() {
    echo
    echo "compose status:"
    compose ps -a || true
    echo
    echo "compose logs:"
    compose logs --no-color --tail=200 || true
}

cleanup() {
    status=$?
    if [ "$status" -ne 0 ]; then
        print_failure_context
    fi
    compose down >/dev/null 2>&1 || true
    exit "$status"
}

wait_for_json() {
    name="$1"
    url="$2"
    jq_filter="$3"
    auth_header="${4:-}"
    deadline=$(( $(date +%s) + timeout_seconds ))

    while [ "$(date +%s)" -lt "$deadline" ]; do
        if [ -n "$auth_header" ]; then
            body="$(curl -fsS -H "Authorization: Bearer $auth_header" "$url" 2>/dev/null || true)"
        else
            body="$(curl -fsS "$url" 2>/dev/null || true)"
        fi
        if [ -n "$body" ] && printf '%s' "$body" | jq -e "$jq_filter" >/dev/null 2>&1; then
            echo "ok: $name"
            return 0
        fi
        sleep 2
    done

    echo "timed out waiting for $name at $url" >&2
    if [ -n "${body:-}" ]; then
        echo "last response: $body" >&2
    fi
    return 1
}

expect_status() {
    name="$1"
    url="$2"
    want_status="$3"
    status="$(curl -s -o /dev/null -w '%{http_code}' "$url")"
    if [ "$status" != "$want_status" ]; then
        echo "failed: $name: status $status, want $want_status" >&2
        return 1
    fi
    echo "ok: $name"
}

wait_for_dev_issuer_token() {
    deadline=$(( $(date +%s) + timeout_seconds ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        token="$(curl -fsS -X POST "$dev_issuer_base/token" 2>/dev/null | jq -r '.token // empty' 2>/dev/null || true)"
        if [ -n "$token" ]; then
            echo "$token"
            return 0
        fi
        sleep 2
    done
    echo "timed out waiting for a dev issuer token at $dev_issuer_base/token" >&2
    return 1
}

wait_for_text() {
    name="$1"
    url="$2"
    needle="$3"
    deadline=$(( $(date +%s) + timeout_seconds ))

    while [ "$(date +%s)" -lt "$deadline" ]; do
        body="$(curl -fsS "$url" 2>/dev/null || true)"
        if [ -n "$body" ] && printf '%s' "$body" | grep -F "$needle" >/dev/null 2>&1; then
            echo "ok: $name"
            return 0
        fi
        sleep 2
    done

    echo "timed out waiting for $name at $url" >&2
    return 1
}

require_command docker
require_command curl
require_command jq

trap cleanup EXIT INT TERM

echo "validating compose configuration"
compose config >/dev/null

echo "starting Atlas dev stack"
compose up --build -d

wait_for_json "api health" "$api_base/healthz" '.status == "ok"'
wait_for_json "current cluster" "$api_base/api/v1/clusters/current" \
    '.name == "reef-baremetal-healthy" and .type == "bare-metal"'
wait_for_json "cluster health" "$api_base/api/v1/clusters/current/health" \
    '.status == "HEALTH_OK" and .summary == "cluster is healthy"'
wait_for_json "current OSDs" "$api_base/api/v1/clusters/current/osds" \
    'length >= 1 and all(.[]; has("id") and has("host") and has("up") and has("in"))'
wait_for_json "current hosts" "$api_base/api/v1/clusters/current/hosts" \
    'length >= 1 and all(.[]; has("name") and (.name != ""))'
wait_for_json "current storage devices" "$api_base/api/v1/clusters/current/storage-devices" \
    'length >= 2 and all(.[]; has("host") and has("serial")) and (map(select(has("osdId"))) | length >= 1)'
wait_for_json "current daemons" "$api_base/api/v1/clusters/current/daemons" \
    'length >= 3 and any(.[]; .type == "mon" and .status == "running")'
wait_for_json "current pools" "$api_base/api/v1/clusters/current/pools" \
    'length >= 1 and all(.[]; .name != "" and (.type == "replicated" or .type == "erasure"))'
wait_for_json "inventory sync runs" "$api_base/api/v1/inventory-sync-runs" \
    'length >= 1 and .[0].provider == "fake" and .[0].status == "succeeded"'
wait_for_json "alert evaluation runs" "$api_base/api/v1/alert-evaluation-runs" \
    'length >= 1 and .[0].provider == "fake" and .[0].status == "succeeded" and .[0].alertsEvaluated == 1'
wait_for_json "cases" "$api_base/api/v1/cases" \
    'any(.[]; .title == "CephOSDDown on osd=1" and .status == "detected" and .severity == "high") and any(.[]; .title == "Review weekly capacity trend" and .status == "triaged")'
detected_case_id="$(curl -fsS "$api_base/api/v1/cases" | jq -r '.[] | select(.title == "CephOSDDown on osd=1") | .id')"
wait_for_json "detected case timeline" "$api_base/api/v1/cases/${detected_case_id}/timeline" \
    'length == 1 and .[0].type == "case_detected" and .[0].payload.signal == "CEPH_OSD_DOWN" and .[0].payload.clusterFsid == "00000000-0000-4000-8000-000000000102"'
seed_case_id="$(curl -fsS "$api_base/api/v1/cases" | jq -r '.[] | select(.title == "Review weekly capacity trend") | .id')"
wait_for_json "seed case timeline" "$api_base/api/v1/cases/${seed_case_id}/timeline" \
    'length == 2 and .[0].type == "case_detected" and .[1].type == "case_triaged"'

wait_for_text "web UI" "$web_base/" '<title>Atlas</title>'
wait_for_json "web API proxy" "$web_base/healthz" '.status == "ok"'

dev_token="$(wait_for_dev_issuer_token)"
expect_status "me without token is rejected" "$api_base/api/v1/me" 401
wait_for_json "me with dev token" "$api_base/api/v1/me" \
    '.subject == "dev-operator" and .displayName == "Dev Operator"' "$dev_token"

echo "Atlas dev stack smoke check passed"
