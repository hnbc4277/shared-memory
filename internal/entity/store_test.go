package entity_test

import (
	"testing"

	"sharedmemory/internal/entity"
)

func TestSplitAlias(t *testing.T) {
	cases := []struct {
		body, wantCanon, wantAlias string
	}{
		{"OpenSearch（又名OS）", "OpenSearch", "OS"},
		{"Redis (aka RDB)", "Redis", "RDB"},
		{"Foo/Bar", "Foo", "Bar"},
		{"plain", "plain", ""},
	}
	for _, tc := range cases {
		canon, alias := entity.SplitAlias(tc.body)
		if canon != tc.wantCanon || alias != tc.wantAlias {
			t.Fatalf("%q: got %q/%q want %q/%q", tc.body, canon, alias, tc.wantCanon, tc.wantAlias)
		}
	}
}
