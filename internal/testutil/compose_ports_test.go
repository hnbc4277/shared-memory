package testutil_test

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var forbiddenHostPorts = []string{"5432", "6379", "9000", "9200", "6333"}

var requiredHostPorts = []string{"15432", "16379", "19000", "19200", "16333"}

func loadComposeDoc(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCompose_DoesNotBindDefaultHostPorts(t *testing.T) {
	doc := loadComposeDoc(t)
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

func TestCompose_BindsRequiredProjectHostPorts(t *testing.T) {
	doc := loadComposeDoc(t)
	services := doc["services"].(map[string]any)

	found := make(map[string]bool, len(requiredHostPorts))
	for _, port := range requiredHostPorts {
		found[port] = false
	}

	for name, raw := range services {
		svc := raw.(map[string]any)
		ports, ok := svc["ports"].([]any)
		if !ok {
			continue
		}
		for _, p := range ports {
			host := hostSidePort(p.(string))
			if host == "" {
				t.Fatalf("service %s has unparseable port mapping: %v", name, p)
			}
			if _, required := found[host]; required {
				found[host] = true
			}
		}
	}

	for _, port := range requiredHostPorts {
		if !found[port] {
			t.Fatalf("compose must expose required host port %s", port)
		}
	}
}

func TestCompose_NamedProjectNetwork(t *testing.T) {
	doc := loadComposeDoc(t)
	networks, ok := doc["networks"].(map[string]any)
	if !ok {
		t.Fatal("compose must define top-level networks")
	}
	raw, ok := networks["sharedmemory_net"]
	if !ok {
		t.Fatal("networks must include sharedmemory_net")
	}
	net, ok := raw.(map[string]any)
	if !ok {
		t.Fatal("sharedmemory_net must be a mapping")
	}
	if net["name"] != "sharedmemory_net" {
		t.Fatalf("sharedmemory_net.name must be sharedmemory_net, got %v", net["name"])
	}

	services := doc["services"].(map[string]any)
	for name, rawSvc := range services {
		svc := rawSvc.(map[string]any)
		nets, ok := svc["networks"].([]any)
		if !ok {
			t.Fatalf("service %s must attach to sharedmemory_net", name)
		}
		attached := false
		for _, n := range nets {
			if n.(string) == "sharedmemory_net" {
				attached = true
				break
			}
		}
		if !attached {
			t.Fatalf("service %s must attach to sharedmemory_net", name)
		}
	}
}

func TestCompose_VolumeKeysUseProjectPrefix(t *testing.T) {
	doc := loadComposeDoc(t)
	volumes, ok := doc["volumes"].(map[string]any)
	if !ok || len(volumes) == 0 {
		t.Fatal("compose must define named volumes with sharedmemory_ prefix")
	}
	for name := range volumes {
		if !strings.HasPrefix(name, "sharedmemory_") {
			t.Fatalf("volume key %q must have sharedmemory_ prefix", name)
		}
	}

	services := doc["services"].(map[string]any)
	for name, raw := range services {
		svc := raw.(map[string]any)
		vols, ok := svc["volumes"].([]any)
		if !ok {
			continue
		}
		for _, v := range vols {
			spec := v.(string)
			volName := strings.SplitN(spec, ":", 2)[0]
			if strings.HasPrefix(volName, "./") || strings.HasPrefix(volName, "/") {
				continue
			}
			if !strings.HasPrefix(volName, "sharedmemory_") {
				t.Fatalf("service %s volume %q must use sharedmemory_ prefixed named volume", name, spec)
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
