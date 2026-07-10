package config

import (
	"encoding/json"
	"testing"
)

func TestConfigReadWriteRedactsSecretValues(t *testing.T) {
	svc := NewService(WithWorkDir("/tmp/work"))

	if _, err := svc.WriteValue(ValueWriteParams{Key: "api.token", Value: json.RawMessage(`"secret-value"`)}); err != nil {
		t.Fatalf("WriteValue secret: %v", err)
	}
	if _, err := svc.WriteValue(ValueWriteParams{Key: "reasoning.effort", Value: json.RawMessage(`"high"`)}); err != nil {
		t.Fatalf("WriteValue effort: %v", err)
	}
	read := svc.Read(ReadParams{})
	secret := findEntry(t, read.Entries, "api.token")
	if !secret.Redacted || string(secret.Value) != "null" {
		t.Fatalf("secret entry = %#v", secret)
	}
	visible := findEntry(t, read.Entries, "reasoning.effort")
	if visible.Redacted || string(visible.Value) != `"high"` {
		t.Fatalf("visible entry = %#v", visible)
	}
	if string(read.Values["api.token"]) != "null" {
		t.Fatalf("secret value leaked in values map: %s", read.Values["api.token"])
	}
}

func TestConfigBatchRequirementsAndEnvironment(t *testing.T) {
	svc := NewService(
		WithWorkDir("/tmp/work"),
		WithEnvLookup(mapEnv(map[string]string{
			"ANTHROPIC_API_KEY": "secret",
			"SHELL":             "/bin/zsh",
			"HOME":              "/Users/example",
		})),
	)

	batch, err := svc.BatchWrite(BatchWriteParams{
		Values: map[string]json.RawMessage{
			"provider.default": json.RawMessage(`"anthropic"`),
		},
		Entries: []ValueWriteParams{{Key: "custom.flag", Value: json.RawMessage(`true`)}},
	})
	if err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}
	if len(batch.Entries) != 2 || string(batch.Values["custom.flag"]) != "true" {
		t.Fatalf("batch response = %#v", batch)
	}

	reqs := svc.Requirements()
	if !requirementSatisfied(reqs.Requirements, "anthropic.apiKey") {
		t.Fatalf("requirements did not reflect env status: %#v", reqs.Requirements)
	}
	if !requirementSatisfied(reqs.Requirements, "workspace.root") {
		t.Fatalf("workspace requirement not satisfied: %#v", reqs.Requirements)
	}

	added, err := svc.AddEnvironment(EnvironmentAddParams{
		ID:      "staging",
		Name:    "Staging",
		WorkDir: "/tmp/staging",
		Variables: map[string]string{
			"OPENAI_API_KEY": "not-returned",
		},
	})
	if err != nil {
		t.Fatalf("AddEnvironment: %v", err)
	}
	if added.Environment.ID != "staging" || len(added.Environment.Variables) != 1 || !added.Environment.Variables[0].Redacted {
		t.Fatalf("added environment = %#v", added.Environment)
	}
	info := svc.EnvironmentInfo()
	if info.CurrentID != "current" || len(info.Environments) != 2 {
		t.Fatalf("environment info = %#v", info)
	}
}

func TestConfigCatalogsAndExperimentalFeatureSet(t *testing.T) {
	svc := NewService()

	if len(svc.CollaborationModes().Modes) == 0 {
		t.Fatal("CollaborationModes returned no modes")
	}
	if len(svc.PermissionProfiles().Profiles) == 0 {
		t.Fatal("PermissionProfiles returned no profiles")
	}
	set, err := svc.SetExperimentalFeature(ExperimentalFeatureSetParams{ID: "websocket-transport", Enabled: false})
	if err != nil {
		t.Fatalf("SetExperimentalFeature: %v", err)
	}
	if set.Feature.Enabled {
		t.Fatalf("feature was not disabled: %#v", set.Feature)
	}
	reload := svc.ReloadMCPServers()
	if reload.Reloaded || reload.Status != "no-op" || reload.Count != 1 {
		t.Fatalf("reload response = %#v", reload)
	}
}

func findEntry(t *testing.T, entries []Entry, key string) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Key == key {
			return entry
		}
	}
	t.Fatalf("entry %q not found in %#v", key, entries)
	return Entry{}
}

func requirementSatisfied(requirements []Requirement, id string) bool {
	for _, requirement := range requirements {
		if requirement.ID == id {
			return requirement.Satisfied
		}
	}
	return false
}

func mapEnv(values map[string]string) EnvLookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
