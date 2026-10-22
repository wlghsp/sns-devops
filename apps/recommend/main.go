package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	serviceName = "sns-recommend"

	maxRequestBodyBytes = 1 << 20
)

type config struct {
	port     string
	fastMs   int
	slowMs   int
	jitterMs int
	otlpURL  string
}

type rankRequest struct {
	UserID  int64   `json:"userId"`
	PostIDs []int64 `json:"postIds"`
}

type rankResponse struct {
	UserID        int64   `json:"userId"`
	Segment       string  `json:"segment"`
	RankedPostIDs []int64 `json:"rankedPostIds"`
	TookMs        int64   `json:"tookMs"`
}

func main() {
	cfg := loadConfig()

	shutdown, err := initTracer(cfg.otlpURL)
	if err != nil {
		slog.Error("트레이서 초기화 실패", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("POST /v1/rank", otelhttp.NewHandler(rankHandler(cfg), "POST /v1/rank"))

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("추천 서비스 시작",
			"port", cfg.port, "fastMs", cfg.fastMs, "slowMs", cfg.slowMs, "otlp", cfg.otlpURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("서버 종료", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	shutdown(ctx)
	slog.Info("추천 서비스 정상 종료")
}

func rankHandler(cfg config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		span := trace.SpanFromContext(ctx)

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

		var req rankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "본문을 읽을 수 없습니다")
			return
		}
		if req.UserID <= 0 {
			writeError(w, http.StatusBadRequest, "userId 가 필요합니다")
			return
		}

		segment := segmentOf(req.UserID)
		span.SetAttributes(
			attribute.Int64("user.id", req.UserID),
			attribute.String("user.segment", segment),
			attribute.Int("rank.candidates", len(req.PostIDs)),
		)

		start := time.Now()
		score(ctx, cfg, segment)
		ranked := rank(req.UserID, req.PostIDs)
		took := time.Since(start).Milliseconds()

		span.SetAttributes(attribute.Int64("rank.took_ms", took))

		logger(ctx).Info("랭킹 완료",
			"userId", req.UserID, "segment", segment,
			"candidates", len(req.PostIDs), "tookMs", took)

		writeJSON(w, http.StatusOK, rankResponse{
			UserID:        req.UserID,
			Segment:       segment,
			RankedPostIDs: ranked,
			TookMs:        took,
		})
	})
}

func score(ctx context.Context, cfg config, segment string) {
	_, span := otel.Tracer(serviceName).Start(ctx, "model-inference")
	defer span.End()

	base := cfg.fastMs
	if segment == "beta" {
		base = cfg.slowMs
	}
	delay := base
	if cfg.jitterMs > 0 {
		delay += rand.Intn(cfg.jitterMs)
	}
	span.SetAttributes(
		attribute.String("model.variant", segment),
		attribute.Int("model.delay_ms", delay),
	)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// 실제 추천 모델 대신 사용자별로 후보 순서를 회전합니다.
func rank(userID int64, postIDs []int64) []int64 {
	ranked := make([]int64, 0, len(postIDs))
	if len(postIDs) == 0 {
		return ranked
	}
	offset := int(userID % int64(len(postIDs)))
	ranked = append(ranked, postIDs[offset:]...)
	return append(ranked, postIDs[:offset]...)
}

func segmentOf(userID int64) string {
	if userID%3 == 0 {
		return "beta"
	}
	return "ga"
}

func logger(ctx context.Context) *slog.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return slog.Default()
	}
	return slog.With("trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
}

func initTracer(otlpURL string) (func(context.Context), error) {
	ctx := context.Background()

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(otlpURL+"/v1/traces"),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.TraceContext{})

	return func(ctx context.Context) { _ = tp.Shutdown(ctx) }, nil
}

func loadConfig() config {
	return config{
		port:     env("PORT", "8080"),
		fastMs:   envInt("FAST_MS", 40),
		slowMs:   envInt("SLOW_MS", 260),
		jitterMs: envInt("JITTER_MS", 60),
		otlpURL:  env("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
