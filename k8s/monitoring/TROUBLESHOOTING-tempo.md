# Tempo 설치 트러블슈팅 (Pending → CrashLoopBackOff)

2026-10-22, 07강 Tempo 설치 중 발생한 문제 두 가지와 해결 기록입니다.
서로 다른 원인이 연달아 나타났습니다.

## 요약

| 순서 | 증상 | 근본 원인 | 해결 |
|---|---|---|---|
| 1 | `tempo-0` Pending | inotify 인스턴스 한도(128) 초과 → kube-proxy 크래시 → local-path-provisioner 불통 → PVC 미생성 | inotify 한도 상향 + 관련 파드 재시작 |
| 2 | `tempo-0` CrashLoopBackOff | Tempo 2.10.8이 거부하는 `opencensus` 키가 설정에 남음 (차트 1.24.4의 null 키 처리) | `config` 템플릿을 덮어써 receivers 직접 지정 |
| 3 | values 수정 후에도 계속 CrashLoopBackOff | StatefulSet 롤링 업데이트가 크래시 파드 때문에 진행 안 됨 | `kubectl delete pod tempo-0` |

---

## 문제 1. Pending: PVC가 만들어지지 않음

### 증상

```
tempo-0   0/1   Pending
```

파드 이벤트는 비어 있었고, PVC 이벤트에 아래 메시지가 반복되었습니다.

```
Waiting for a volume to be created either by the external provisioner 'rancher.io/local-path' ...
```

### 원인 연쇄

```
inotify 인스턴스 한도 128 초과
  → control-plane의 kube-proxy가 시작 실패
     ("fsnotify watcher init: too many open files", 30회 재시작)
  → control-plane 노드의 Service 네트워크 불통 (coredns도 0/1)
  → 같은 노드의 local-path-provisioner가 API 서버(10.96.0.1:443)에 접속 못 함
     (i/o timeout, CrashLoopBackOff)
  → PVC(storage-tempo-0) 프로비저닝 불가
  → tempo-0 스케줄링 불가 (Pending)
```

- `too many open files`는 파일 디스크립터 부족이 아니라 **inotify 인스턴스 개수 한도** 초과를 뜻합니다.
- `fs.inotify.max_user_instances`는 **사용자(UID) 단위로 커널 전체에서 합산**됩니다.
  Kind 노드(control-plane 1 + worker 2)는 같은 Docker VM 커널을 공유하고 모두 root로 동작하므로,
  세 노드와 그 위의 모든 파드가 128개를 나눠 씁니다.
- Prometheus, Grafana(사이드카 다수), Loki, OTel Collector에 Tempo까지 더해지며 한도에 도달한 것으로 추정합니다.
  (VM 전체 사용량은 직접 측정하지 못한 정황 추정입니다.)

### 해결

```bash
# 1. 한도 상향 (Kind 노드는 privileged라 노드에서 실행해도 커널 전체에 적용됨)
docker exec sns-cluster-control-plane sysctl -w \
  fs.inotify.max_user_instances=1024 fs.inotify.max_user_watches=524288

# 2. 죽은 파드 재시작
kubectl delete pod -n kube-system <crashloop 중인 kube-proxy 파드>
kubectl rollout restart deploy/coredns -n kube-system
kubectl delete pod -n local-path-storage -l app=local-path-provisioner

# 3. PVC가 Bound 되는지 확인
kubectl get pvc -n monitoring
```

### 재발 방지

- 이 sysctl 값은 Docker Desktop을 재시작하면 초기화될 수 있습니다. 같은 증상이 다시 나면 위 1~3을 반복합니다.
- 진단 순서: 파드 `describe`(이벤트 없음) → PVC `describe` → provisioner 로그 → kube-proxy 로그.

---

## 문제 2. CrashLoopBackOff: Tempo가 opencensus 키를 거부

### 증상

```
error running Tempo ... 'receivers' unknown type: "opencensus" for id: "opencensus"
(valid values: [jaeger zipkin otlp kafka])
```

### 원인

- Tempo 2.10.8은 opencensus receiver를 더 이상 지원하지 않습니다.
- `tempo-values.yaml`에 `receivers.opencensus: null`을 넣어 제거하려 했습니다.
- Helm은 사용자 값의 `null`로 **차트 기본값의 키를 삭제**합니다.
  그런데 Helm 차트 `grafana/tempo` 1.24.4는 기본값도 `opencensus:`(null)이라 삭제 대상이 없어,
  렌더링된 ConfigMap에 `opencensus: null`이 그대로 남았습니다.
- Tempo는 값이 null이어도 **키 이름 자체**가 알 수 없는 타입이면 설정 로드에 실패합니다.

확인 방법:

```bash
kubectl get cm tempo -n monitoring -o jsonpath='{.data.tempo\.yaml}' | grep opencensus
helm template tempo grafana/tempo --version 1.24.4 -n monitoring -f k8s/monitoring/tempo-values.yaml
```

### 해결

`k8s/monitoring/tempo-values.yaml`에서 `tempo.receivers` 블록을 삭제하고,
차트의 `config` 템플릿을 덮어써 receivers를 OTLP만 직접 지정했습니다.
(jaeger receiver도 함께 제외. 07강은 OTLP만 사용)

---

## 문제 3. values를 고쳐도 파드가 그대로

### 증상

`helm upgrade`는 성공했는데 `kubectl rollout status`가 계속 대기하고, 파드는 이전 설정으로 크래시.
ConfigMap은 이미 갱신되었지만(`opencensus` 0건) 파드의 리비전 해시는 이전 값이었습니다.

```
currentRevision: tempo-5f9fd8c4dc   updateRevision: tempo-66c8684497
```

### 원인

StatefulSet의 롤링 업데이트는 기존 파드가 Ready여야 다음으로 진행합니다.
크래시 중인 `tempo-0`이 Ready가 아니어서 새 리비전으로 교체되지 않았습니다.

### 해결

```bash
kubectl delete pod tempo-0 -n monitoring
```

StatefulSet이 새 리비전으로 파드를 재생성합니다. PVC는 유지되므로 데이터는 지워지지 않습니다.
이후 `tempo-0`이 `1/1 Running`이 되었습니다.

---

## 별개로 확인된 사항

- 한때 `docker.io` 접속이 `i/o timeout`으로 실패했습니다(`alpine` 이미지 pull 실패).
  Tempo 이미지는 이후 정상적으로 받아졌지만, 네트워크가 불안정하면 새 이미지 pull에서 `ImagePullBackOff`가 날 수 있습니다.
- 이 환경(Docker VM 약 5.8GiB)에서는 Tempo 라이트 values(`tempo-values.yaml`) 기준으로 진행했습니다.
  full(`tempo-values-full.yaml`)을 쓰려면 `config` 템플릿에 `metrics_generator` 블록을 추가해야 합니다.
