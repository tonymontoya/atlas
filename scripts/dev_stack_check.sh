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

# expect_post_status asserts a POST's status code; the body and optional
# bearer token are passed through unchanged.
expect_post_status() {
    name="$1"
    url="$2"
    want_status="$3"
    body="$4"
    token="${5:-}"
    if [ -n "$token" ]; then
        status="$(curl -s -o /dev/null -w '%{http_code}' -X POST \
            -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
            -d "$body" "$url")"
    else
        status="$(curl -s -o /dev/null -w '%{http_code}' -X POST \
            -H "Content-Type: application/json" -d "$body" "$url")"
    fi
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
cluster_fsid="00000000-0000-4000-8000-000000000101"
wait_for_json "cluster index" "$api_base/api/v1/clusters" \
    '.total >= 1 and any(.clusters[]; .fsid == "00000000-0000-4000-8000-000000000101" and .name == "reef-baremetal-healthy" and .clusterType == "bare-metal" and .healthStatus == "HEALTH_OK")'
wait_for_json "cluster index search" "$api_base/api/v1/clusters?q=reef-baremetal-healthy" \
    '.total == 1 and .clusters[0].fsid == "00000000-0000-4000-8000-000000000101"'
wait_for_json "cluster health" "$api_base/api/v1/clusters/$cluster_fsid/health" \
    '.status == "HEALTH_OK" and .summary == "cluster is healthy"'
wait_for_json "current OSDs" "$api_base/api/v1/clusters/$cluster_fsid/osds" \
    'length >= 1 and all(.[]; has("id") and has("host") and has("up") and has("in"))'
wait_for_json "current hosts" "$api_base/api/v1/clusters/$cluster_fsid/hosts" \
    'length >= 1 and all(.[]; has("name") and (.name != ""))'
wait_for_json "current storage devices" "$api_base/api/v1/clusters/$cluster_fsid/storage-devices" \
    'length >= 2 and all(.[]; has("host") and has("serial")) and (map(select(has("osdId"))) | length >= 1)'
wait_for_json "current daemons" "$api_base/api/v1/clusters/$cluster_fsid/daemons" \
    'length >= 3 and any(.[]; .type == "mon" and .status == "running")'
wait_for_json "current pools" "$api_base/api/v1/clusters/$cluster_fsid/pools" \
    'length >= 1 and all(.[]; .name != "" and (.type == "replicated" or .type == "erasure"))'
expect_status "unknown cluster health is 404" "$api_base/api/v1/clusters/00000000-0000-4000-8000-0000000009ff/health" 404
wait_for_json "inventory sync runs" "$api_base/api/v1/inventory-sync-runs" \
    'length >= 1 and .[0].provider == "fake" and .[0].status == "succeeded"'
wait_for_json "alert evaluation runs" "$api_base/api/v1/alert-evaluation-runs" \
    'length >= 1 and .[0].provider == "fake" and .[0].status == "succeeded" and .[0].alertsEvaluated == 1'
wait_for_json "cases" "$api_base/api/v1/cases" \
    'any(.[]; .title == "CephOSDDown on osd=1" and .status == "detected" and .severity == "high") and any(.[]; .title == "Review weekly capacity trend" and .status == "triaged")'
wait_for_json "cluster-filtered cases" "$api_base/api/v1/cases?cluster=00000000-0000-4000-8000-000000000102" \
    'any(.[]; .title == "CephOSDDown on osd=1") and (all(.[]; .clusterFsid == "00000000-0000-4000-8000-000000000102"))'
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

# The workflow loop (ADR-0022): attach the Replace OSD Workflow to a fresh
# Case, approve the Approval Gate, complete the human Task, and watch the
# fake agent drive the instance to terminal succeeded. Every run attaches
# to its own freshly created Case, so reruns against a persistent volume
# stay green.
expect_post_status "attach workflow without token is rejected" \
    "$api_base/api/v1/cases/1/workflows" 401 \
    '{"workflowId":"replace-osd","workflowVersion":1}'
expect_post_status "approve gate without token is rejected" \
    "$api_base/api/v1/workflow-instances/1/approvals" 401 \
    '{"gateId":"approve-destroy"}'
expect_post_status "complete task without token is rejected" \
    "$api_base/api/v1/workflow-instances/1/task-completions" 401 \
    '{"taskId":"replace-device"}'

loop_case_id="$(curl -fsS -X POST \
    -H "Authorization: Bearer $dev_token" -H "Content-Type: application/json" \
    -d '{"title":"dev-stack-check Replace OSD loop","summary":"workflow loop probe case","severity":"low","clusterFsid":"00000000-0000-4000-8000-000000000102"}' \
    "$api_base/api/v1/cases" | jq -r '.id')"
loop_instance_id="$(curl -fsS -X POST \
    -H "Authorization: Bearer $dev_token" -H "Content-Type: application/json" \
    -d '{"workflowId":"replace-osd","workflowVersion":1}' \
    "$api_base/api/v1/cases/${loop_case_id}/workflows" | jq -r '.id')"

wait_for_json "attached instance pauses at the approval gate" \
    "$api_base/api/v1/cases/${loop_case_id}/workflows" \
    'length == 1 and .[0].state == "waiting_for_approval" and .[0].currentStep == "approve-destroy"'
wait_for_json "attached jobs rest pending at the gate" \
    "$api_base/api/v1/workflow-instances/${loop_instance_id}/jobs" \
    '([.[].stepId]) == ["collect-evidence","destroy-osd","verify-osd"] and all(.[]; .state == "pending")'

expect_post_status "gate approval is recorded" \
    "$api_base/api/v1/workflow-instances/${loop_instance_id}/approvals" 201 \
    '{"gateId":"approve-destroy","reason":"dev-stack-check"}' "$dev_token"

wait_for_json "approved instance pauses at the operator task" \
    "$api_base/api/v1/cases/${loop_case_id}/workflows" \
    'length == 1 and .[0].state == "waiting_for_operator" and .[0].currentStep == "replace-device"'
wait_for_json "jobs before the task ran through the fake agent" \
    "$api_base/api/v1/workflow-instances/${loop_instance_id}/jobs" \
    '([.[].state]) == ["succeeded","succeeded","pending"]'

expect_post_status "task completion is recorded" \
    "$api_base/api/v1/workflow-instances/${loop_instance_id}/task-completions" 201 \
    '{"taskId":"replace-device","note":"dev-stack-check device swap"}' "$dev_token"

wait_for_json "completed instance reaches terminal succeeded" \
    "$api_base/api/v1/cases/${loop_case_id}/workflows" \
    'length == 1 and .[0].state == "succeeded" and .[0].finishedAt != null'
wait_for_json "all jobs succeeded" \
    "$api_base/api/v1/workflow-instances/${loop_instance_id}/jobs" \
    'length == 3 and all(.[]; .state == "succeeded")'
wait_for_json "workflow loop timeline" \
    "$api_base/api/v1/cases/${loop_case_id}/timeline" \
    'length == 8 and .[0].type == "case_detected" and ([.[] | select(.type == "workflow_attached")] | length == 1) and ([.[] | select(.type == "workflow_state_changed") | .payload.newState]) == ["running","waiting_for_approval","running","waiting_for_operator","running","succeeded"]'

echo "Atlas dev stack smoke check passed"
