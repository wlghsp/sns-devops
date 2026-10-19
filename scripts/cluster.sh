#!/usr/bin/env bash
# 학습하지 않을 때 Kind 클러스터를 멈추고, 다시 시작할 때 이어서 켭니다.
# colima(Docker)는 그대로 두고 클러스터 노드 컨테이너만 제어합니다. 클러스터 상태는 보존됩니다.
set -euo pipefail

CLUSTER="${KIND_CLUSTER:-sns-cluster}"
FILTER="label=io.x-k8s.kind.cluster=${CLUSTER}"

require_docker() {
    if ! docker info >/dev/null 2>&1; then
        echo "Docker에 연결할 수 없습니다. colima가 꺼져 있다면 'colima start'를 먼저 실행하세요." >&2
        exit 1
    fi
}

nodes() {
    docker ps -a --filter "$FILTER" --format '{{.Names}}'
}

start() {
    require_docker
    local all
    all="$(nodes)"
    if [ -z "$all" ]; then
        echo "'${CLUSTER}' 클러스터가 없습니다. scripts/part-2/run.sh로 먼저 생성하세요." >&2
        exit 1
    fi

    # control-plane을 먼저 켜야 worker가 붙을 수 있습니다.
    echo "$all" | grep -- '-control-plane$' | xargs docker start >/dev/null
    echo "$all" | grep -v -- '-control-plane$' | xargs -r docker start >/dev/null

    echo "노드가 Ready 될 때까지 기다립니다..."
    kubectl wait --for=condition=Ready node --all --timeout=180s
    kubectl get nodes
}

stop() {
    require_docker
    local running
    running="$(docker ps --filter "$FILTER" --format '{{.Names}}')"
    if [ -z "$running" ]; then
        echo "실행 중인 '${CLUSTER}' 노드가 없습니다."
        return
    fi
    # worker를 먼저 끄고 control-plane을 마지막에 끕니다.
    echo "$running" | grep -v -- '-control-plane$' | xargs -r docker stop >/dev/null
    echo "$running" | grep -- '-control-plane$' | xargs docker stop >/dev/null
    echo "'${CLUSTER}' 클러스터를 멈췄습니다."
}

status() {
    require_docker
    docker ps -a --filter "$FILTER" --format 'table {{.Names}}\t{{.Status}}'
}

case "${1:-}" in
    start) start ;;
    stop) stop ;;
    status) status ;;
    *) echo "사용법: $0 {start|stop|status}" >&2; exit 2 ;;
esac
