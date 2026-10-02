package semantic_test

import (
	"context"
	"math"
	"testing"

	"sharedmemory/internal/semantic"
)

func cosine(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i] * b[i])
	}
	return dot
}

func TestHashEmbedder_SimilarTextCloser(t *testing.T) {
	ctx := context.Background()
	e := semantic.HashEmbedder{}
	q, _ := e.Embed(ctx, "数据库使用 PostgreSQL")
	near, _ := e.Embed(ctx, "项目数据库使用 PostgreSQL 16")
	far, _ := e.Embed(ctx, "今天天气很好适合散步")
	if len(q) != semantic.HashDim {
		t.Fatalf("dim = %d", len(q))
	}
	var norm float64
	for _, v := range q {
		norm += float64(v * v)
	}
	if math.Abs(norm-1) > 1e-4 {
		t.Fatalf("not normalized: %f", norm)
	}
	if cosine(q, near) <= cosine(q, far) {
		t.Fatalf("near %f must exceed far %f", cosine(q, near), cosine(q, far))
	}
}

func TestHeuristic_Extract(t *testing.T) {
	got, err := semantic.Heuristic{}.Extract(context.Background(), "user",
		"我偏好使用 Go 写后端。我们决定采用方案二。项目使用 Docker Compose 隔离。", true)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, e := range got {
		kinds[e.Kind] = e.Scope
	}
	if kinds[semantic.KindPreference] != semantic.ScopeGlobal {
		t.Fatalf("preference must be global: %+v", got)
	}
	if kinds[semantic.KindDecision] != semantic.ScopeProject {
		t.Fatalf("decision must be project: %+v", got)
	}
	if kinds[semantic.KindFact] != semantic.ScopeProject {
		t.Fatalf("fact must be project: %+v", got)
	}
}
