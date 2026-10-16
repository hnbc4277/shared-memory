package semantic_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/semantic"
)

type fakeAccountSettings struct {
	settings semantic.Settings
	apiKey   string
}

func (f *fakeAccountSettings) Get(_ context.Context, _ uuid.UUID) (semantic.Settings, error) {
	return f.settings, nil
}

func (f *fakeAccountSettings) PlainAPIKey(_ context.Context, _ uuid.UUID) (string, error) {
	return f.apiKey, nil
}

func TestResolverForAccount_HashFallback(t *testing.T) {
	accountID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	r := &semantic.Resolver{
		Settings: &fakeAccountSettings{},
	}

	got, err := r.ForAccount(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "hash" {
		t.Fatalf("source=%q want hash", got.Source)
	}
	if got.Dim != semantic.HashDim {
		t.Fatalf("dim=%d want %d", got.Dim, semantic.HashDim)
	}
	if got.Collection != "" {
		t.Fatalf("collection=%q want empty", got.Collection)
	}
	if got.Protocol != "hash" {
		t.Fatalf("protocol=%q want hash", got.Protocol)
	}
	if got.ReindexRequired {
		t.Fatal("reindex should be false")
	}
	if _, ok := got.Embedder.(semantic.HashEmbedder); !ok {
		t.Fatalf("embedder type %T want HashEmbedder", got.Embedder)
	}
}

func TestResolverForAccount_DeploymentTEI(t *testing.T) {
	accountID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	r := &semantic.Resolver{
		Settings:       &fakeAccountSettings{},
		DeployProtocol: "tei",
		DeployURL:      "http://embed:8080",
		DeployModel:    "bge-small-zh-v1.5",
		DeployDim:      512,
	}

	got, err := r.ForAccount(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "deployment" {
		t.Fatalf("source=%q want deployment", got.Source)
	}
	if got.Dim != 512 {
		t.Fatalf("dim=%d want 512", got.Dim)
	}
	compact := strings.ReplaceAll(accountID.String(), "-", "")[:12]
	if !strings.Contains(got.Collection, compact) {
		t.Fatalf("collection=%q missing compact prefix %q", got.Collection, compact)
	}
	if !strings.HasSuffix(got.Collection, "_512") {
		t.Fatalf("collection=%q want suffix _512", got.Collection)
	}
	if got.Embedder == nil {
		t.Fatal("embedder nil")
	}
	if got.Embedder.Dim() != 512 {
		t.Fatalf("embedder dim=%d want 512", got.Embedder.Dim())
	}
}

func TestResolverForAccount_AccountOpenAI(t *testing.T) {
	accountID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	r := &semantic.Resolver{
		Settings: &fakeAccountSettings{
			settings: semantic.Settings{
				Enabled:  true,
				Protocol: "openai",
				BaseURL:  "http://llm:8080/v1",
				Model:    "text-embedding-3-large",
				Dim:      2048,
			},
			apiKey: "sk",
		},
	}

	got, err := r.ForAccount(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "account" {
		t.Fatalf("source=%q want account", got.Source)
	}
	if got.Dim != 2048 {
		t.Fatalf("dim=%d want 2048", got.Dim)
	}
	if got.Embedder == nil {
		t.Fatal("embedder nil")
	}
	if got.Embedder.Dim() != 2048 {
		t.Fatalf("embedder dim=%d want 2048", got.Embedder.Dim())
	}
	if got.ReindexRequired {
		t.Fatal("reindex should be false")
	}
}

func TestResolverForAccount_AccountOpenAIReindexRequired(t *testing.T) {
	accountID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	r := &semantic.Resolver{
		Settings: &fakeAccountSettings{
			settings: semantic.Settings{
				Enabled:         true,
				Protocol:        "openai",
				BaseURL:         "http://llm:8080/v1",
				Model:           "text-embedding-3-large",
				Dim:             2048,
				ReindexRequired: true,
			},
			apiKey: "sk",
		},
	}

	got, err := r.ForAccount(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "account" {
		t.Fatalf("source=%q want account", got.Source)
	}
	if !got.ReindexRequired {
		t.Fatal("reindex should be true")
	}
	if got.Embedder == nil {
		t.Fatal("embedder nil")
	}
}
