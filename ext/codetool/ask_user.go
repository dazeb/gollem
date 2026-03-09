package codetool

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// AskUserQuestion is a single question with multiple-choice options.
type AskUserQuestion struct {
	Text    string   `json:"text" jsonschema:"description=The question to ask"`
	Options []string `json:"options" jsonschema:"description=2-4 answer options"`
}

// AskUserAnswer is the user's response to a question.
type AskUserAnswer struct {
	QuestionIndex int    `json:"question_index"`
	Selected      string `json:"selected"`
}

// AskUserFunc presents questions to the user and returns their answers.
type AskUserFunc func(ctx context.Context, questions []AskUserQuestion) ([]AskUserAnswer, error)

// AskUserParams are the parameters for the ask_user tool.
type AskUserParams struct {
	Questions []AskUserQuestion `json:"questions" jsonschema:"description=1-4 questions to ask the user\, each with a question text and 2-4 answer options"`
}

const (
	minQuestions   = 1
	maxQuestions   = 4
	minOptionsPerQ = 2
	maxOptionsPerQ = 4
)

// AskUser creates a tool that asks the user structured questions.
func AskUser(askFn AskUserFunc) core.Tool {
	if askFn == nil {
		return core.Tool{}
	}
	return core.FuncTool[AskUserParams](
		"ask_user",
		"Ask the user 1-4 structured multiple-choice questions. Each question has 2-4 options. The user can also provide a custom answer beyond the listed options.",
		func(ctx context.Context, params AskUserParams) (string, error) {
			if len(params.Questions) < minQuestions || len(params.Questions) > maxQuestions {
				return "", &core.ModelRetryError{Message: fmt.Sprintf("must provide %d-%d questions, got %d", minQuestions, maxQuestions, len(params.Questions))}
			}
			for i, q := range params.Questions {
				if strings.TrimSpace(q.Text) == "" {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("question %d has empty text", i+1)}
				}
				if len(q.Options) < minOptionsPerQ || len(q.Options) > maxOptionsPerQ {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("question %d must have %d-%d options, got %d", i+1, minOptionsPerQ, maxOptionsPerQ, len(q.Options))}
				}
			}
			answers, err := askFn(ctx, params.Questions)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			for _, a := range answers {
				if a.QuestionIndex >= 0 && a.QuestionIndex < len(params.Questions) {
					fmt.Fprintf(&b, "Q: %s\nAnswer: %s\n\n", params.Questions[a.QuestionIndex].Text, a.Selected)
				}
			}
			return b.String(), nil
		},
	)
}
