package recall

import (
	"testing"

	"github.com/google/uuid"
)

func TestExpandQuery_NoEntities(t *testing.T) {
	if got := expandQuery(nil, nil, uuid.Nil, nil, "OS"); got != "OS" {
		t.Fatalf("got %q", got)
	}
}
