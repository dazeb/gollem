package protocol

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTypeScriptBindingGoldenIsDeterministic(t *testing.T) {
	generated, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	second, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript second call: %v", err)
	}
	if !bytes.Equal(generated, second) {
		t.Fatal("TypeScript generation is nondeterministic")
	}
	golden, err := os.ReadFile(filepath.Join("typescript", "gollem_appserver_protocol.ts"))
	if err != nil {
		t.Fatalf("read TypeScript golden: %v", err)
	}
	if !bytes.Equal(generated, golden) {
		t.Fatal("TypeScript binding is stale; run go generate ./appserver/protocol")
	}
}

func TestTypeScriptFixtureGoldenIsDeterministic(t *testing.T) {
	for _, name := range []string{"runtime_wire_v1", "initialize_wire_v1"} {
		t.Run(name, func(t *testing.T) {
			fixture, err := os.ReadFile(filepath.Join("testdata", name+".json"))
			if err != nil {
				t.Fatalf("read %s fixture: %v", name, err)
			}
			generated, err := MarshalTypeScriptFixture(fixture)
			if err != nil {
				t.Fatalf("MarshalTypeScriptFixture: %v", err)
			}
			golden, err := os.ReadFile(filepath.Join("typescript", "testdata", name+".ts"))
			if err != nil {
				t.Fatalf("read TypeScript fixture golden: %v", err)
			}
			if !bytes.Equal(generated, golden) {
				t.Fatal("TypeScript fixture is stale; run go generate ./appserver/protocol")
			}
		})
	}
}

func TestTypeScriptBindingCoversDefinitionsAndBindings(t *testing.T) {
	generated, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	source := string(generated)
	defs := JSONSchema()["$defs"].(Schema)
	for name := range defs {
		if !strings.Contains(source, "export type "+name+" =") {
			t.Errorf("TypeScript binding missing definition %s", name)
		}
	}
	for _, binding := range WireTypeBindings() {
		if !strings.Contains(source, "\""+binding.Method+"\":") {
			t.Errorf("TypeScript binding missing method %s", binding.Method)
		}
	}
	for _, binding := range ItemPayloadBindings() {
		want := "\"" + binding.Kind + "\": " + binding.Type
		if !strings.Contains(source, want) {
			t.Errorf("TypeScript binding missing item payload %s", want)
		}
	}
	for _, declaration := range []string{
		"export interface MethodParamsByName",
		"export interface MethodResultsByName",
		"export type BoundRequest<",
		"export type BoundNotification<",
		"export type BoundResponse<",
		"export type KnownTimelineItem =",
	} {
		if !strings.Contains(source, declaration) {
			t.Errorf("TypeScript binding missing %q", declaration)
		}
	}
}

func TestTypeScriptFixtureRejectsEnvelopeSurfaceMismatch(t *testing.T) {
	fixture := []byte(`{
		"protocolVersion":"gollem.appserver.v0",
		"schemaVersion":"gollem.appserver.schema.v1",
		"cases":[{
			"name":"invalid-initialized-request",
			"surface":"client-notification",
			"method":"initialized",
			"envelope":"request",
			"message":{"id":1,"method":"initialized"}
		}]
	}`)
	_, err := MarshalTypeScriptFixture(fixture)
	if err == nil || !strings.Contains(err.Error(), `envelope "request" is incompatible with surface "client-notification"`) {
		t.Fatalf("MarshalTypeScriptFixture error = %v", err)
	}
}

func TestTypeScriptFixtureTypeChecksWhenCompilerIsAvailable(t *testing.T) {
	if _, err := exec.LookPath("tsc"); err != nil {
		t.Skip("tsc is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tsc", "--project", filepath.Join("typescript", "tsconfig.json"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("TypeScript fixture failed to compile: %v\n%s", err, output)
	}
}
