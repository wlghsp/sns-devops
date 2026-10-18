# k8s/

Kind 클러스터 설정, Kubernetes 매니페스트, Helm values를 관리합니다.
학습자 기준으로 "복붙 가능한 실행 순서 + 검증 포인트"를 먼저 제공합니다.

이 문서는 전체 강의의 최종 안내이며, `part-2-kind-deployment`부터 마지막 `part-*` 브랜치까지 같은 내용을 사용합니다.
`main`에는 기본 안내와 실행 스크립트가 먼저 제공되어 있고, 이 문서는 02강부터 포함돼요. 아래 설정 파일은 해당 단원에서 작성한 뒤 명령을 실행합니다.
완료본을 참고할 때도 현재 단원까지 준비된 파일과 환경에 맞는 명령만 실행하세요.

명령은 별도 표시가 없으면 `sns-devops` 루트 폴더 기준입니다. 설치와 설정은 아래 명령을 직접 실행하고, 08강과 09강의 재현 및 분석에는 스크립트를 사용해요.

## 1) 사전 준비

macOS는 [Homebrew](https://brew.sh/ko/)를 먼저 설치한 뒤 터미널에서, Windows는 PowerShell에서 아래 명령을 실행하세요.
Windows에서 `winget`이 없다면 Microsoft Store에서 **앱 설치 관리자**를 설치하면 됩니다.

| 도구 | macOS | Windows |
| --- | --- | --- |
| Docker Desktop | [공식 설치 파일](https://docs.docker.com/desktop/setup/install/mac-install/)에서 내 Mac의 칩(Apple Silicon 또는 Intel)에 맞는 파일을 받아 설치하고 실행해요. | [공식 설치 파일](https://docs.docker.com/desktop/setup/install/windows-install/)을 실행하고, WSL 2 사용 옵션을 선택해 설치한 뒤 실행합니다. |
| `kubectl` | `brew install kubernetes-cli` | `winget install -e --id Kubernetes.kubectl` |
| `kind` | `brew install kind` | `winget install -e --id Kubernetes.kind` |
| `helm` | `brew install helm` | `winget install -e --id Helm.Helm` |

설치가 끝나면 터미널을 다시 열고 아래 명령으로 확인해요.

확인 명령:

```bash
docker --version
kubectl version --client
kind version
helm version
```

### 프로파일 선택

이 저장소는 두 가지 프로파일을 제공합니다. **기본은 lite** 입니다.

| | lite (기본) | full (강의 영상 기준) |
| --- | --- | --- |
| Docker Desktop 할당 메모리 | 4GB | 8GB |
| 노드 구성 | 단일 노드 | control-plane 1 + worker 2 |
| node-exporter | 없음 | 있음 |
| Grafana 기본 대시보드 | 없음 (커뮤니티 대시보드 직접 임포트) | 있음 |
| Tempo metrics-generator | 없음 | 있음 |
| 05강 이후 ArgoCD | 제거하고 진행 | 유지 |

강의 영상은 full 로 촬영했습니다. 노트북 메모리가 8GB 이하라면 lite 로 진행하세요.
실습 내용과 명령은 같고, 화면에 보이는 Pod 개수와 일부 선택 실습만 달라집니다.

**Docker Desktop 메모리 할당 확인**: Settings > Resources > Memory limit 에서
lite 는 4GB 이상, full 은 8GB 이상으로 설정하세요. 기본값 그대로 두면
06강 이후 Pod 이 `OOMKilled` 로 반복 재시작합니다.

아래 명령은 lite 기준입니다. `-full` 파일은 lite 와의 **차이만** 담고 있어서,
full 로 진행할 때는 기본 values 뒤에 이어서 붙입니다.

| | lite (기본) | full |
| --- | --- | --- |
| 클러스터 | `-f k8s/kind/kind-config.yaml` | `-f k8s/kind/kind-config-full.yaml` |
| Prometheus | `-f k8s/monitoring/kube-prometheus-values.yaml` | 왼쪽에 `-f k8s/monitoring/kube-prometheus-values-full.yaml` 추가 |
| Tempo | `-f k8s/monitoring/tempo-values.yaml` | 왼쪽에 `-f k8s/monitoring/tempo-values-full.yaml` 추가 |

나머지 values 파일은 두 프로파일이 공유합니다.

### 차트 버전

설치 명령에 버전을 고정해 두었습니다. 강의 영상과 같은 결과를 보려면 그대로 사용하세요.

| 차트 | 버전 | 앱 버전 |
| --- | --- | --- |
| Gateway API CRD | v1.6.1 (standard) | 해당 없음 |
| traefik/traefik | 41.2.0 | v3.7.10 |
| argo/argo-cd | 10.3.3 | v3.5.1 |
| prometheus-community/kube-prometheus-stack | 88.3.0 | v0.93.0 |
| grafana/loki | 7.3.0 | 3.6.12 |
| open-telemetry/opentelemetry-collector | 0.169.0 | 0.158.0 |
| grafana/tempo | 1.24.4 | 2.10.8 (values 에서 지정) |

## 2) 구조

```text
k8s/
├── kind/
│   ├── kind-config.yaml           # lite 기본 (control-plane only)
│   └── kind-config-full.yaml      # full (control-plane + 2 workers)
├── gateway/
│   ├── traefik-values.yaml        # Traefik Helm values (Gateway API provider)
│   ├── sns-app.yaml               # sns, rustfs HTTPRoute (02강)
│   ├── monitoring.yaml            # grafana, prometheus HTTPRoute (05강)
│   ├── loki.yaml                  # loki HTTPRoute (06강)
│   └── tempo.yaml                 # tempo HTTPRoute (07강)
├── argocd/
│   ├── argocd-values.yaml         # ArgoCD Helm values
│   └── application.yaml           # ArgoCD Application CRD
├── monitoring/
│   ├── alertmanager-values.yaml   # AlertManager Slack 연동
│   ├── kube-prometheus-values.yaml       # lite 기본
│   ├── kube-prometheus-values-full.yaml  # full
│   ├── loki-values.yaml           # Loki Helm values (공통)
│   ├── otel-collector-values.yaml # 06강 로그 파일 수집, 07강부터 트레이스 추가
│   ├── tempo-values.yaml          # lite 기본
│   ├── tempo-values-full.yaml     # full
│   └── servicemonitor.yaml        # sns-app ServiceMonitor
└── sns-app/
    ├── namespace.yaml
    ├── postgres.yaml
    ├── redis.yaml
    ├── rustfs.yaml
    ├── recommend.yaml            # 추천 서비스 (07강)
    └── app.yaml
```

## 3) 클러스터 생성/삭제

```bash
# lite (기본)
kind create cluster --config k8s/kind/kind-config.yaml

# full 로 진행한다면
# kind create cluster --config k8s/kind/kind-config-full.yaml

kubectl get nodes
```

lite 는 노드가 1개, full 은 3개로 보입니다.

삭제:

```bash
kind delete cluster --name sns-cluster
```

## 4) 설치 순서 (복붙용)

강의 진행 순서와 같습니다. lite 기준이고, full 은 표시된 곳에서 values 를 하나 더 붙입니다.

### 4-1. Gateway API CRD 와 Traefik (02강)

Gateway API 는 Kubernetes 코어에 포함되지 않은 CRD 라 먼저 설치해야 합니다.

```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.6.1/standard-install.yaml
```

```bash
helm repo add traefik https://traefik.github.io/charts
helm repo update

helm install traefik traefik/traefik --version 41.2.0 -n traefik --create-namespace \
  -f k8s/gateway/traefik-values.yaml
```

차트가 `GatewayClass` 와 기본 `Gateway`(`traefik` 네임스페이스의 `traefik-gateway`)를
함께 만듭니다. 확인해 보세요.

```bash
kubectl get gatewayclass
kubectl get gateway -n traefik
```

`GatewayClass` 는 `ACCEPTED=True`, `Gateway` 는 `PROGRAMMED=True` 여야 합니다.

HTTPRoute 는 대상 네임스페이스가 만들어진 뒤에 적용합니다. 순서를 바꾸면
`namespaces "..." not found` 로 실패해요.

### 4-2. 애플리케이션 배포 (02강)

```bash
# 로컬에서 빌드한 앱 이미지를 kind 노드에 적재합니다.
kind load docker-image springboot-sns:latest --name sns-cluster

kubectl apply -f k8s/sns-app/namespace.yaml
kubectl apply -f k8s/sns-app/postgres.yaml \
  -f k8s/sns-app/redis.yaml \
  -f k8s/sns-app/rustfs.yaml
kubectl apply -f k8s/sns-app/app.yaml
kubectl apply -f k8s/gateway/sns-app.yaml
```

### 4-3. ArgoCD (04강)

```bash
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update

helm install argocd argo/argo-cd --version 10.3.3 -n argocd --create-namespace \
  -f k8s/argocd/argocd-values.yaml
kubectl apply -f k8s/argocd/application.yaml
```

lite 로 진행한다면 04강을 마친 뒤 여기서 ArgoCD 를 제거하고 다음 단계로 갑니다.
04강까지의 실습 결과는 Git 저장소에 남아 있고, 05강 이후에는 ArgoCD 가 필요하지 않습니다.

```bash
helm uninstall argocd -n argocd
kubectl delete namespace argocd
```

메모리를 500MB 가량 확보하는 것 외에 이유가 하나 더 있습니다. Application 이
`selfHeal: true` 라서, 켜둔 채로 08강의 `kubectl scale deployment sns-app --replicas=0` 을
실행하면 60초 안에 복제본이 자동 복구되어 장애 상황이 재현되지 않아요.

### 4-4. Prometheus + Grafana (05강)

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm install prometheus prometheus-community/kube-prometheus-stack --version 88.3.0 \
  -n monitoring --create-namespace \
  -f k8s/monitoring/kube-prometheus-values.yaml

kubectl apply -f k8s/monitoring/servicemonitor.yaml
kubectl apply -f k8s/gateway/monitoring.yaml

```

full 은 마지막 `-f` 뒤에 `-f k8s/monitoring/kube-prometheus-values-full.yaml` 을 추가합니다.

HTTPRoute 는 대상 Service 를 만드는 강의에서 각각 추가합니다. 05강은 Grafana 와
Prometheus 두 개이고, Loki 와 Tempo 는 06~07강에서 붙입니다.

```bash
kubectl get httproute -n monitoring
```

`Accepted` 와 `ResolvedRefs` 가 모두 True 여야 합니다.

### 4-5. Loki + OTel Collector (06강)

06강에서는 sns-app의 JSON 콘솔 로그를 노드 파일에서 읽어 Loki로 보냅니다.
앱은 로그 출력, 노드별 OTel Collector는 수집과 전송을 담당해요.
05강의 Grafana를 그대로 사용하고 Loki 데이터소스를 추가합니다.

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm repo update

helm install loki grafana/loki --version 7.3.0 -n monitoring \
  -f k8s/monitoring/loki-values.yaml

helm install otel-collector open-telemetry/opentelemetry-collector --version 0.169.0 \
  -n monitoring -f k8s/monitoring/otel-collector-values.yaml

kubectl apply -f k8s/gateway/loki.yaml
```

### 4-6. Tempo + OTel Collector 확장 (07강)

먼저 Go 추천 서비스를 로컬에서 빌드하고 Kind에 배포합니다. 별도 이미지 레지스트리는 필요 없어요.

```bash
docker build -t sns-recommend:latest apps/recommend
kind load docker-image sns-recommend:latest --name sns-cluster
kubectl apply -f k8s/sns-app/app.yaml
kubectl apply -f k8s/sns-app/recommend.yaml
kubectl rollout status deployment/sns-recommend -n sns --timeout=120s
```

07강에서는 Tempo를 설치하고, 06강에서 설치한 Collector에 OTLP 트레이스 수신을 추가합니다.
앱의 트레이스는 OTLP로 보내고, 로그는 계속 파일에서 수집해요.
`part-7-traces` 브랜치의 values 파일로 기존 Collector를 갱신하면 로그 수집을 유지하면서
트레이스를 Tempo로 보낼 수 있어요.

```bash
helm install tempo grafana/tempo --version 1.24.4 -n monitoring \
  -f k8s/monitoring/tempo-values.yaml
```

Tempo를 full로 진행하면 위 설치 명령에 `-f k8s/monitoring/tempo-values-full.yaml`을
추가합니다. metrics-generator가 켜져 트레이스 기반 메트릭을 추가로 생성해요.

```bash
helm upgrade otel-collector open-telemetry/opentelemetry-collector --version 0.169.0 \
  -n monitoring -f k8s/monitoring/otel-collector-values.yaml

kubectl apply -f k8s/gateway/tempo.yaml
```

### 4-7. RED 경보와 Slack 연동 (08강)

`sns-app`의 08강 코드를 main에 머지하고, CI와 ArgoCD로 배포가 완료되면 진행해요.
Slack 앱의 Incoming Webhooks에서 알림 채널을 선택하고 URL을 발급합니다. URL은 Git에 저장하지 않아요.

```bash
# sns-devops 폴더에서 최초 1회 등록
kubectl create secret generic slack-webhook -n monitoring \
  --from-literal=url='실제_WEBHOOK_URL'

# Slack 설정 적용
helm upgrade prometheus prometheus-community/kube-prometheus-stack --version 88.3.0 \
  -n monitoring --reuse-values -f k8s/monitoring/alertmanager-values.yaml

# 경보 규칙 적용
kubectl apply -f k8s/monitoring/alertrules.yaml
```

**재현:** `sns-app` 폴더에서 아래 명령을 하나씩 실행합니다. 각 명령은 준비 30초, 재현 3분, 복구 3분 순서예요.

```bash
./scripts/part-8/run.sh rate     # 초당 요청량 5건 초과
./scripts/part-8/run.sh error    # 5xx 비율 10% 초과
./scripts/part-8/run.sh latency  # p95 응답 시간 1초 초과
```

최근 2분의 값이 기준을 30초 넘으면 경보가 발생합니다. Prometheus Alerts와 Slack에서 발생과 해소를 확인한 뒤 다음 명령을 실행해요.

### 4-8. AI Agent로 장애 분석 (09강)

09강 앱을 main에 머지하고 CI와 ArgoCD로 배포합니다. 추천 서비스와 메트릭, 로그, 트레이스 수집이 준비되어 있어야 해요.
`sns-devops`에는 09강의 `skills/obsctl`과 장애 분석 스킬을 준비합니다. 추가 Helm 설치는 없으며, 기존 배포 상태를 확인해요.

```bash
kubectl rollout status deployment/sns-app -n sns --timeout=180s
kubectl rollout status deployment/sns-recommend -n sns --timeout=120s
kubectl get pods -n monitoring
```

```bash
# sns-app 폴더: 장애 분석용 요청 60회
./scripts/part-9/run.sh 60

# sns-devops 폴더: 최근 30분의 관측 데이터 조회
./scripts/part-9/run.sh 30
```

AI Agent에 아래와 같이 요청해요.

> sns-app의 최근 30분 오류 원인을 메트릭, 로그, 트레이스로 조사하고 근거와 대응안을 알려줘. 코드 수정과 배포는 하지 마.

오류가 발생하는 요청 조건과 실패한 호출 구간을 확인합니다.

## 5) 접속과 상태 확인

| 서비스 | 주소 |
| --- | --- |
| SNS App | http://sns.localhost |
| ArgoCD | http://argocd.localhost |
| Grafana | http://grafana.localhost |
| Prometheus Alerts | http://prometheus.localhost/alerts |

```bash
kubectl get pods -A
curl -f http://sns.localhost/actuator/health
```

Pod가 준비되고 앱이 `UP`인지 확인해요. 접속이 안 되면 `kubectl get httproute -A`로 경로 상태를 확인합니다.
