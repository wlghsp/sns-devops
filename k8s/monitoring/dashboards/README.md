# Grafana 대시보드

## SNS App RED

[sns-app-red.json](./sns-app-red.json)은 요청량(req/s), 5xx 오류율(%), 평균 응답 시간(ms)을 보여줍니다. 최근 5분 기준이며 `/actuator/health` 요청만 제외해요.

Grafana **Dashboards → New → Import**에서 JSON을 업로드하고 Prometheus 데이터소스를 선택하세요.

## JVM (Micrometer), 4701

[4701 대시보드](https://grafana.com/grafana/dashboards/4701-jvm-micrometer/)로 힙 메모리, GC, CPU, 스레드를 확인할 수 있습니다. Import 화면에서 ID `4701`을 입력해요.

앱의 기존 `application.yaml`에 아래 설정을 병합하고 재배포한 뒤, 대시보드에서 `application=sns-app`을 선택하세요.

```yaml
management:
  metrics:
    tags:
      application: sns-app
```
