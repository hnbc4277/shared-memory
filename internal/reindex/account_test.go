package reindex

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/semantic"
)

type fakeSettings struct {
	st      semantic.Settings
	upserts int
}

func (f *fakeSettings) Get(context.Context, uuid.UUID) (semantic.Settings, error) {
	return f.st, nil
}

func (f *fakeSettings) Upsert(_ context.Context, _ uuid.UUID, st semantic.Settings, _ ...string) error {
	f.upserts++
	f.st = st
	return nil
}

func TestMarkReindexed_ClearsFlag(t *testing.T) {
	accountID := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	store := &fakeSettings{st: semantic.Settings{
		Enabled: true, Protocol: "openai", BaseURL: "http://embed", Model: "m", Dim: 8,
		ReindexRequired: true, APIKeySet: true,
	}}
	r := &Reindexer{Settings: store}
	res := semantic.Resolved{Protocol: "openai", Model: "m", Dim: 8, BaseURL: "http://embed"}
	if err := r.markReindexed(context.Background(), accountID, res); err != nil {
		t.Fatal(err)
	}
	if store.st.ReindexRequired {
		t.Fatal("flag still set")
	}
	want := semantic.Fingerprint("openai", "m", 8, "http://embed")
	if store.st.ActiveFingerprint != want {
		t.Fatalf("fingerprint=%q want %q", store.st.ActiveFingerprint, want)
	}
	if store.upserts != 1 {
		t.Fatalf("upserts=%d", store.upserts)
	}
}

func TestMarkReindexed_SkipsMissingRow(t *testing.T) {
	store := &fakeSettings{}
	r := &Reindexer{Settings: store}
	if err := r.markReindexed(context.Background(), uuid.New(), semantic.Resolved{}); err != nil {
		t.Fatal(err)
	}
	if store.upserts != 0 {
		t.Fatalf("upserts=%d", store.upserts)
	}
}
