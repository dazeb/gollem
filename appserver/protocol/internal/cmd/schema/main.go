package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fugue-labs/gollem/appserver/protocol"
)

func main() {
	writeGenerated("schema.json", protocol.MarshalJSONSchema)
	writeGenerated(filepath.Join("typescript", "gollem_appserver_protocol.ts"), protocol.MarshalTypeScript)
	for _, name := range []string{"runtime_wire_v1", "initialize_wire_v1", "thread_discovery_wire_v1"} {
		fixture, err := os.ReadFile(filepath.Join("testdata", name+".json"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		writeGenerated(filepath.Join("typescript", "testdata", name+".ts"), func() ([]byte, error) {
			return protocol.MarshalTypeScriptFixture(fixture)
		})
	}
}

func writeGenerated(path string, generate func() ([]byte, error)) {
	data, err := generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
