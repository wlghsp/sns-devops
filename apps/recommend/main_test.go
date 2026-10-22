package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

var testConfig = config{fastMs: 0, slowMs: 0, jitterMs: 0}

func TestSegmentOf(t *testing.T) {
	cases := map[int64]string{1: "ga", 2: "ga", 3: "beta", 4: "ga", 6: "beta", 9: "beta"}
	for userID, want := range cases {
		if got := segmentOf(userID); got != want {
			t.Errorf("userId %d: %q 를 기대했지만 %q", userID, want, got)
		}
	}
}

func TestRankIsDeterministic(t *testing.T) {
	postIDs := []int64{101, 102, 103, 104, 105}

	first := rank(7, postIDs)
	second := rank(7, postIDs)

	if !slices.Equal(first, second) {
		t.Fatalf("같은 입력인데 순서가 다릅니다: %v vs %v", first, second)
	}
}

func TestRankPreservesCandidateSet(t *testing.T) {
	postIDs := []int64{101, 102, 103, 104, 105}

	ranked := rank(7, postIDs)

	if len(ranked) != len(postIDs) {
		t.Fatalf("후보 %d 개를 넣었는데 %d 개가 나왔습니다", len(postIDs), len(ranked))
	}
	got, want := append([]int64(nil), ranked...), append([]int64(nil), postIDs...)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if !slices.Equal(got, want) {
		t.Errorf("후보 집합이 달라졌습니다: %v", ranked)
	}
}

func TestRankDoesNotMutateInput(t *testing.T) {
	postIDs := []int64{101, 102, 103, 104, 105}
	original := append([]int64(nil), postIDs...)

	rank(7, postIDs)

	if !slices.Equal(postIDs, original) {
		t.Errorf("입력 슬라이스가 변경되었습니다: %v", postIDs)
	}
}

func TestRankHandlerResponse(t *testing.T) {
	body, _ := json.Marshal(rankRequest{UserID: 3, PostIDs: []int64{101, 102, 103}})
	rec := httptest.NewRecorder()

	rankHandler(testConfig).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/rank", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("200 을 기대했지만 %d", rec.Code)
	}
	var resp rankResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("응답을 읽을 수 없습니다: %v", err)
	}
	if resp.Segment != "beta" {
		t.Errorf("userId 3 은 beta 여야 하는데 %q", resp.Segment)
	}
	if len(resp.RankedPostIDs) != 3 {
		t.Errorf("후보 3 개를 기대했지만 %v", resp.RankedPostIDs)
	}
}

func TestRankHandlerRejectsMissingUserID(t *testing.T) {
	body, _ := json.Marshal(rankRequest{PostIDs: []int64{101}})
	rec := httptest.NewRecorder()

	rankHandler(testConfig).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/rank", bytes.NewReader(body)))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("400 을 기대했지만 %d", rec.Code)
	}
}

func TestRankEmptyCandidates(t *testing.T) {
	if got := rank(1, nil); got == nil || len(got) != 0 {
		t.Fatalf("빈 후보는 JSON 배열로 반환해야 합니다: %v", got)
	}
}

func TestIncomingTraceparentIsContinued(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	const incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const incomingSpanID = "00f067aa0ba902b7"
	handler := otelhttp.NewHandler(rankHandler(testConfig), "POST /v1/rank",
		otelhttp.WithTracerProvider(provider))
	req := httptest.NewRequest(http.MethodPost, "/v1/rank",
		bytes.NewBufferString(`{"userId":1,"postIds":[101,102]}`))
	req.Header.Set("traceparent", "00-"+incomingTraceID+"-"+incomingSpanID+"-01")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	spans := recorder.Ended()
	if rec.Code != http.StatusOK || len(spans) != 1 {
		t.Fatalf("status=%d spans=%d", rec.Code, len(spans))
	}
	span := spans[0]
	if span.SpanContext().TraceID().String() != incomingTraceID ||
		span.Parent().SpanID().String() != incomingSpanID ||
		span.SpanContext().SpanID().String() == incomingSpanID ||
		span.SpanKind() != trace.SpanKindServer {
		t.Fatalf("서버 span의 부모 자식 관계가 잘못되었습니다: %v", span)
	}
}
