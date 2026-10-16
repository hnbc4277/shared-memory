package index

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/semantic"
)

type mockTextIndex struct {
	hits []Hit
	err  error
}

func (m *mockTextIndex) Ensure(context.Context) error { return nil }
func (m *mockTextIndex) Index(context.Context, Doc) error { return nil }
func (m *mockTextIndex) Search(_ context.Context, _ Query) ([]Hit, error) {
	return m.hits, m.err
}
func (m *mockTextIndex) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *mockTextIndex) DeleteProject(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *mockTextIndex) DeleteAccount(context.Context, uuid.UUID) error { return nil }

type mustNotSearchVector struct {
	t *testing.T
}

func (v *mustNotSearchVector) Ensure(context.Context) error { return nil }
func (v *mustNotSearchVector) Upsert(context.Context, Doc, []float32) error { return nil }
func (v *mustNotSearchVector) Search(context.Context, Query) ([]Hit, error) {
	v.t.Fatal("vector Search must not be called when ReindexRequired")
	return nil, nil
}
func (v *mustNotSearchVector) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (v *mustNotSearchVector) DeleteProject(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (v *mustNotSearchVector) DeleteAccount(context.Context, uuid.UUID) error { return nil }

type fakeAccountSettings struct {
	st semantic.Settings
}

func (f *fakeAccountSettings) Get(_ context.Context, _ uuid.UUID) (semantic.Settings, error) {
	return f.st, nil
}
func (f *fakeAccountSettings) PlainAPIKey(context.Context, uuid.UUID) (string, error) {
	return "", nil
}

func TestSearch_SkipsVectorWhenReindexRequired(t *testing.T) {
	accountID := uuid.New()
	wantID := uuid.New()
	text := &mockTextIndex{hits: []Hit{{ID: wantID, DocType: DocMemory, Score: 1}}}

	resolver := &semantic.Resolver{
		Settings: &fakeAccountSettings{st: semantic.Settings{
			Enabled:         true,
			Protocol:        "tei",
			BaseURL:         "http://embed.test",
			Model:           "test-model",
			Dim:             32,
			ReindexRequired: true,
		}},
	}

	s := &Service{
		Text:     text,
		Vector:   &mustNotSearchVector{t: t},
		Embedder: semantic.HashEmbedder{},
		Resolver: resolver,
	}

	hits, err := s.Search(context.Background(), Query{AccountID: accountID, Text: "q", Limit: 10})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}
	if len(hits) != 1 || hits[0].ID != wantID {
		t.Fatalf("hits = %+v, want one text hit %v", hits, wantID)
	}
}
