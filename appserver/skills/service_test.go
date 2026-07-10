package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceListsPluginsAndSkills(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeFile(t, root, "standalone/SKILL.md", "# Standalone\n\nUseful standalone skill.\n")
	writeFile(t, root, "plugins/example/.codex-plugin/plugin.json", `{"id":"example","name":"Example Plugin","description":"Example plugin","version":"1.2.3"}`)
	writeFile(t, root, "plugins/example/skills/review/SKILL.md", "---\nname: Review Skill\ndescription: Read code carefully.\n---\n# Ignored Heading\n")

	svc := NewService(WithRoot(root))
	skills, err := svc.ListSkills(ctx, ListParams{})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills.Skills) != 2 || len(skills.Data) != 2 || len(skills.Roots) != 1 {
		t.Fatalf("skills response = %#v", skills)
	}
	review := findSkill(t, skills.Skills, "Review Skill")
	if review.PluginID != "example" || review.Description != "Read code carefully." {
		t.Fatalf("review skill = %#v", review)
	}
	standalone := findSkill(t, skills.Skills, "Standalone")
	if standalone.PluginID != "" || standalone.Description != "Useful standalone skill." {
		t.Fatalf("standalone skill = %#v", standalone)
	}

	plugins, err := svc.ListPlugins(ctx, PluginListParams{IncludeSkills: true})
	if err != nil {
		t.Fatalf("ListPlugins: %v", err)
	}
	if len(plugins.Plugins) != 1 {
		t.Fatalf("plugins response = %#v", plugins)
	}
	plugin := plugins.Plugins[0]
	if plugin.ID != "example" || plugin.Version != "1.2.3" || plugin.SkillCount != 1 || len(plugin.Skills) != 1 {
		t.Fatalf("plugin = %#v", plugin)
	}

	readPlugin, err := svc.ReadPlugin(ctx, PluginReadParams{PluginID: "example"})
	if err != nil {
		t.Fatalf("ReadPlugin: %v", err)
	}
	if readPlugin.Plugin.ID != "example" || readPlugin.Manifest["name"] != "Example Plugin" {
		t.Fatalf("read plugin = %#v", readPlugin)
	}

	readSkill, err := svc.ReadPluginSkill(ctx, PluginSkillReadParams{SkillID: review.ID})
	if err != nil {
		t.Fatalf("ReadPluginSkill: %v", err)
	}
	if readSkill.Skill.ID != review.ID || readSkill.Content == "" {
		t.Fatalf("read skill = %#v", readSkill)
	}
}

func TestServiceRejectsEscapingSkillSymlink(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bad"), 0o755); err != nil {
		t.Fatalf("mkdir bad: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "bad", "SKILL.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	svc := NewService(WithRoot(root))
	if _, err := svc.ListSkills(ctx, ListParams{}); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("ListSkills err = %v, want ErrPathOutsideRoot", err)
	}
}

func TestServiceLimitsGenericPluginManifestDepth(t *testing.T) {
	ctx := context.Background()
	pluginRoot := t.TempDir()
	writeFile(t, pluginRoot, "plugin.json", `{"id":"direct","name":"Direct Plugin"}`)
	direct, err := NewService(WithRoot(pluginRoot)).ListPlugins(ctx, PluginListParams{})
	if err != nil {
		t.Fatalf("ListPlugins direct: %v", err)
	}
	if len(direct.Plugins) != 1 || direct.Plugins[0].ID != "direct" {
		t.Fatalf("direct plugins = %#v", direct.Plugins)
	}

	collectionRoot := t.TempDir()
	writeFile(t, collectionRoot, "example/plugin.json", `{"id":"example","name":"Example Plugin"}`)
	writeFile(t, collectionRoot, "nested/too-deep/plugin.json", `{"id":"ignored","name":"Ignored Plugin"}`)
	collection, err := NewService(WithRoot(collectionRoot)).ListPlugins(ctx, PluginListParams{})
	if err != nil {
		t.Fatalf("ListPlugins collection: %v", err)
	}
	if len(collection.Plugins) != 1 || collection.Plugins[0].ID != "example" {
		t.Fatalf("collection plugins = %#v", collection.Plugins)
	}
}

func TestServiceMissingReadsReturnTypedErrors(t *testing.T) {
	ctx := context.Background()
	svc := NewService(WithRoot(t.TempDir()))
	if _, err := svc.ReadPlugin(ctx, PluginReadParams{PluginID: "missing"}); !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("ReadPlugin err = %v, want ErrPluginNotFound", err)
	}
	if _, err := svc.ReadPluginSkill(ctx, PluginSkillReadParams{SkillID: "missing"}); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("ReadPluginSkill err = %v, want ErrSkillNotFound", err)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func findSkill(t *testing.T, skills []Skill, name string) Skill {
	t.Helper()
	for _, skill := range skills {
		if skill.Name == name {
			return skill
		}
	}
	t.Fatalf("skill %q not found in %#v", name, skills)
	return Skill{}
}
