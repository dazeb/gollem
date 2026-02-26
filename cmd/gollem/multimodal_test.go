package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImagePathCandidatesFromPrompt(t *testing.T) {
	got := imagePathCandidatesFromPrompt(`Find best move from "board.png" and /tmp/ref.jpg, ignore notes.txt`)
	if len(got) != 2 {
		t.Fatalf("expected 2 image candidates, got %d (%v)", len(got), got)
	}
	if got[0] != "board.png" {
		t.Fatalf("first candidate = %q, want board.png", got[0])
	}
	if got[1] != "/tmp/ref.jpg" {
		t.Fatalf("second candidate = %q, want /tmp/ref.jpg", got[1])
	}
}

func TestDetectPromptImagePartsExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.png")
	if err := os.WriteFile(path, []byte{0x89, 0x50, 0x4E, 0x47, 0x0A}, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	parts := detectPromptImageParts("Solve this from board.png", dir)
	if len(parts) != 1 {
		t.Fatalf("expected 1 image part, got %d", len(parts))
	}
	if parts[0].MIMEType != "image/png" {
		t.Fatalf("mime type = %q, want image/png", parts[0].MIMEType)
	}
	if !strings.HasPrefix(parts[0].URL, "data:image/png;base64,") {
		t.Fatalf("expected data URL with png prefix, got %q", parts[0].URL)
	}
}

func TestDetectPromptImagePartsCueFallbackSingleImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "position.jpeg"), []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("n/a"), 0o644); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	parts := detectPromptImageParts("What is the best move from this image?", dir)
	if len(parts) != 1 {
		t.Fatalf("expected fallback to attach exactly one image, got %d", len(parts))
	}
	if parts[0].MIMEType != "image/jpeg" {
		t.Fatalf("mime type = %q, want image/jpeg", parts[0].MIMEType)
	}
}

func TestDetectPromptImagePartsCueFallbackSkipsMultipleImages(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte{1}, 0o644); err != nil {
		t.Fatalf("write a.png: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.jpg"), []byte{2}, 0o644); err != nil {
		t.Fatalf("write b.jpg: %v", err)
	}

	parts := detectPromptImageParts("Use the image to solve this", dir)
	if len(parts) != 0 {
		t.Fatalf("expected no fallback attachment with multiple root images, got %d", len(parts))
	}
}

func TestDetectPromptImagePartsRespectsMaxBytes(t *testing.T) {
	t.Setenv("GOLLEM_PROMPT_IMAGE_MAX_BYTES", "4")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "board.png"), []byte{1, 2, 3, 4, 5}, 0o644); err != nil {
		t.Fatalf("write board.png: %v", err)
	}

	parts := detectPromptImageParts("Solve board.png", dir)
	if len(parts) != 0 {
		t.Fatalf("expected oversized image to be skipped, got %d parts", len(parts))
	}
}

func TestDetectPromptImagePartsRejectsPathTraversal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatalf("write secret image: %v", err)
	}

	prompt := "Analyze ../outside/secret.png"
	parts := detectPromptImageParts(prompt, root)
	if len(parts) != 0 {
		t.Fatalf("expected traversal path to be rejected, got %d parts", len(parts))
	}
}

func TestDetectPromptImagePartsRejectsAbsoluteOutsideWorkDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "abs.png")
	if err := os.WriteFile(target, []byte{4, 5, 6}, 0o644); err != nil {
		t.Fatalf("write abs image: %v", err)
	}

	parts := detectPromptImageParts("Analyze "+target, root)
	if len(parts) != 0 {
		t.Fatalf("expected absolute outside path to be rejected, got %d parts", len(parts))
	}
}
