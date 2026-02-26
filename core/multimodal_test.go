package core

import (
	"context"
	"testing"
	"time"
)

func TestImagePart_Interface(t *testing.T) {
	var _ ModelRequestPart = ImagePart{}
	p := ImagePart{URL: "https://example.com/img.png", MIMEType: "image/png", Detail: "auto"}
	if p.requestPartKind() != "image" {
		t.Fatal("wrong kind")
	}
}

func TestAudioPart_Interface(t *testing.T) {
	var _ ModelRequestPart = AudioPart{}
	p := AudioPart{URL: "https://example.com/audio.mp3", MIMEType: "audio/mp3"}
	if p.requestPartKind() != "audio" {
		t.Fatal("wrong kind")
	}
}

func TestDocumentPart_Interface(t *testing.T) {
	var _ ModelRequestPart = DocumentPart{}
	p := DocumentPart{URL: "https://example.com/doc.pdf", MIMEType: "application/pdf", Title: "My Doc"}
	if p.requestPartKind() != "document" {
		t.Fatal("wrong kind")
	}
}

func TestBinaryContent(t *testing.T) {
	data := []byte("hello world")
	result := BinaryContent(data, "text/plain")
	if result != "data:text/plain;base64,aGVsbG8gd29ybGQ=" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestMultimodalRequest(t *testing.T) {
	req := ModelRequest{
		Parts: []ModelRequestPart{
			UserPromptPart{Content: "What's in this image?"},
			ImagePart{URL: "https://example.com/img.png", MIMEType: "image/png"},
		},
		Timestamp: time.Now(),
	}
	if len(req.Parts) != 2 {
		t.Fatal("expected 2 parts")
	}
}

func TestMultimodalSerialization(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	messages := []ModelMessage{
		ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "Describe this image and document", Timestamp: now},
				ImagePart{URL: "https://example.com/photo.jpg", MIMEType: "image/jpeg", Detail: "high", Timestamp: now},
				AudioPart{URL: "data:audio/mp3;base64,AAAA", MIMEType: "audio/mp3", Timestamp: now},
				DocumentPart{URL: "https://example.com/doc.pdf", MIMEType: "application/pdf", Title: "Report", Timestamp: now},
			},
			Timestamp: now,
		},
	}

	data, err := MarshalMessages(messages)
	if err != nil {
		t.Fatalf("MarshalMessages failed: %v", err)
	}

	got, err := UnmarshalMessages(data)
	if err != nil {
		t.Fatalf("UnmarshalMessages failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("message count = %d, want 1", len(got))
	}

	req, ok := got[0].(ModelRequest)
	if !ok {
		t.Fatal("expected ModelRequest")
	}
	if len(req.Parts) != 4 {
		t.Fatalf("parts count = %d, want 4", len(req.Parts))
	}

	// Verify ImagePart round-trip.
	img, ok := req.Parts[1].(ImagePart)
	if !ok {
		t.Fatalf("part[1]: expected ImagePart, got %T", req.Parts[1])
	}
	if img.URL != "https://example.com/photo.jpg" {
		t.Errorf("ImagePart.URL = %q", img.URL)
	}
	if img.MIMEType != "image/jpeg" {
		t.Errorf("ImagePart.MIMEType = %q", img.MIMEType)
	}
	if img.Detail != "high" {
		t.Errorf("ImagePart.Detail = %q", img.Detail)
	}

	// Verify AudioPart round-trip.
	aud, ok := req.Parts[2].(AudioPart)
	if !ok {
		t.Fatalf("part[2]: expected AudioPart, got %T", req.Parts[2])
	}
	if aud.URL != "data:audio/mp3;base64,AAAA" {
		t.Errorf("AudioPart.URL = %q", aud.URL)
	}
	if aud.MIMEType != "audio/mp3" {
		t.Errorf("AudioPart.MIMEType = %q", aud.MIMEType)
	}

	// Verify DocumentPart round-trip.
	doc, ok := req.Parts[3].(DocumentPart)
	if !ok {
		t.Fatalf("part[3]: expected DocumentPart, got %T", req.Parts[3])
	}
	if doc.URL != "https://example.com/doc.pdf" {
		t.Errorf("DocumentPart.URL = %q", doc.URL)
	}
	if doc.Title != "Report" {
		t.Errorf("DocumentPart.Title = %q", doc.Title)
	}
}

func TestWithInitialRequestPartsRun(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model)

	image := ImagePart{
		URL:      "data:image/png;base64,AAAA",
		MIMEType: "image/png",
		Detail:   "high",
	}

	_, err := agent.Run(context.Background(), "describe this board", WithInitialRequestParts(image))
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 model call, got %d", len(calls))
	}
	if len(calls[0].Messages) != 1 {
		t.Fatalf("expected 1 message in first call, got %d", len(calls[0].Messages))
	}

	req, ok := calls[0].Messages[0].(ModelRequest)
	if !ok {
		t.Fatalf("expected ModelRequest, got %T", calls[0].Messages[0])
	}
	if len(req.Parts) != 2 {
		t.Fatalf("expected 2 request parts, got %d", len(req.Parts))
	}

	userPart, ok := req.Parts[0].(UserPromptPart)
	if !ok {
		t.Fatalf("part[0]: expected UserPromptPart, got %T", req.Parts[0])
	}
	if userPart.Content != "describe this board" {
		t.Fatalf("unexpected user content: %q", userPart.Content)
	}

	imagePart, ok := req.Parts[1].(ImagePart)
	if !ok {
		t.Fatalf("part[1]: expected ImagePart, got %T", req.Parts[1])
	}
	if imagePart.URL != image.URL || imagePart.MIMEType != image.MIMEType || imagePart.Detail != image.Detail {
		t.Fatalf("unexpected image part: %+v", imagePart)
	}
}

func TestWithInitialRequestPartsIter(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model)

	image := ImagePart{
		URL:      "data:image/png;base64,BBBB",
		MIMEType: "image/png",
	}

	iter := agent.Iter(context.Background(), "analyze image", WithInitialRequestParts(image))
	_, err := iter.Next()
	if err != nil {
		t.Fatalf("iter next failed: %v", err)
	}

	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 model call, got %d", len(calls))
	}
	req, ok := calls[0].Messages[0].(ModelRequest)
	if !ok {
		t.Fatalf("expected ModelRequest, got %T", calls[0].Messages[0])
	}
	if len(req.Parts) != 2 {
		t.Fatalf("expected 2 request parts, got %d", len(req.Parts))
	}
	if _, ok := req.Parts[0].(UserPromptPart); !ok {
		t.Fatalf("part[0]: expected UserPromptPart, got %T", req.Parts[0])
	}
	if got, ok := req.Parts[1].(ImagePart); !ok || got.URL != image.URL {
		t.Fatalf("part[1]: expected image URL %q, got %#v", image.URL, req.Parts[1])
	}
}
