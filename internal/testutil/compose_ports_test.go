package testutil_test

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var forbiddenHostPorts = []string{"5432", "6379", "9000", "9200", "6333"}

func TestCompose_DoesNotBindDefaultHostPorts(t *testing.T) {
	b, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	services := doc["services"].(map[string]any)
	for name, raw := range services {
		svc := raw.(map[string]any)
		ports, ok := svc["ports"].([]any)
		if !ok {
			continue
		}
		for _, p := range ports {
			s := p.(string)
			for _, port := range forbiddenHostPorts {
				if hasHostPort(s, port) {
					t.Fatalf("service %s binds privileged/common host port: %s", name, s)
				}
			}
		}
	}
}

func hostSidePort(mapping string) string {
	parts := strings.Split(mapping, ":")
	switch len(parts) {
	case 2:
		return parts[0]
	case 3:
		return parts[1]
	default:
		return ""
	}
}

func hasHostPort(mapping, port string) bool {
	return hostSidePort(mapping) == port
}
