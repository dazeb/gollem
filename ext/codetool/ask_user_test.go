package codetool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAskUserValidQuestions(t *testing.T) {
	tool := AskUser(func(_ context.Context, questions []AskUserQuestion) ([]AskUserAnswer, error) {
		if len(questions) != 1 || questions[0].Text != "Which DB?" {
			t.Fatalf("questions=%+v", questions)
		}
		return []AskUserAnswer{{QuestionIndex: 0, Selected: "SQLite"}}, nil
	})
	got := call(t, tool, `{"questions":[{"text":"Which DB?","options":["Postgres","SQLite"]}]}`)
	if !strings.Contains(got, "Which DB?") || !strings.Contains(got, "SQLite") {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestAskUserRejectsInvalidQuestionCounts(t *testing.T) {
	tool := AskUser(func(_ context.Context, _ []AskUserQuestion) ([]AskUserAnswer, error) { return nil, nil })
	if err := callErr(t, tool, `{"questions":[]}`); err == nil {
		t.Fatal("expected retry error")
	}
	if err := callErr(t, tool, `{"questions":[{"text":"q","options":["a","b"]},{"text":"q","options":["a","b"]},{"text":"q","options":["a","b"]},{"text":"q","options":["a","b"]},{"text":"q","options":["a","b"]}]}`); err == nil {
		t.Fatal("expected retry error")
	}
}

func TestAskUserRejectsInvalidQuestionContent(t *testing.T) {
	tool := AskUser(func(_ context.Context, _ []AskUserQuestion) ([]AskUserAnswer, error) { return nil, nil })
	if err := callErr(t, tool, `{"questions":[{"text":"","options":["a","b"]}]}`); err == nil {
		t.Fatal("expected retry error")
	}
	if err := callErr(t, tool, `{"questions":[{"text":"q","options":["a"]}]}`); err == nil {
		t.Fatal("expected retry error")
	}
	if err := callErr(t, tool, `{"questions":[{"text":"q","options":["a","b","c","d","e"]}]}`); err == nil {
		t.Fatal("expected retry error")
	}
}

func TestAskUserPropagatesError(t *testing.T) {
	tool := AskUser(func(_ context.Context, _ []AskUserQuestion) ([]AskUserAnswer, error) { return nil, errors.New("boom") })
	if err := callErr(t, tool, `{"questions":[{"text":"q","options":["a","b"]}]}`); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err=%v", err)
	}
}
