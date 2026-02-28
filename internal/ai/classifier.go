package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/Pankaj3112/labelr/internal/config"
)

type Classifier struct {
	client openai.Client
	model  string
	labels []config.Label
}

type ClassificationResult struct {
	Label string `json:"label"`
}

func NewClassifier(apiKey, baseURL, model string, labels []config.Label) *Classifier {
	opts := []option.RequestOption{}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	} else {
		// For Ollama or no-auth providers
		opts = append(opts, option.WithAPIKey("ollama"))
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	client := openai.NewClient(opts...)

	return &Classifier{
		client: client,
		model:  model,
		labels: labels,
	}
}

func (c *Classifier) Classify(ctx context.Context, from, subject, body string) (string, error) {
	prompt := buildPrompt(from, subject, body, c.labels)
	schema := buildResponseSchema(c.labels)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are an email classifier. You must respond with valid JSON only."),
			openai.UserMessage(prompt),
		},
		Model: c.model,
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "email_classification",
					Schema: schema,
					Strict: openai.Bool(true),
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("AI classification failed: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from AI")
	}

	var result ClassificationResult
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &result); err != nil {
		return "", fmt.Errorf("parsing AI response: %w", err)
	}

	if result.Label == "" {
		return "", fmt.Errorf("AI returned empty label")
	}

	return result.Label, nil
}

// ValidateConnection sends a test message with structured output to verify the API key and model work.
// Retries up to 3 times before returning an error.
func (c *Classifier) ValidateConnection(ctx context.Context) error {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type": "string",
				"enum": []any{"ok"},
			},
		},
		"required":             []string{"status"},
		"additionalProperties": false,
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Respond with status ok."),
			},
			Model: c.model,
			ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
					JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
						Name:   "validation",
						Schema: schema,
						Strict: openai.Bool(true),
					},
				},
			},
		})
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("validation failed after 3 attempts: %w", lastErr)
}

func buildPrompt(from, subject, body string, labels []config.Label) string {
	var sb strings.Builder
	sb.WriteString("Classify this email into exactly one of the provided labels.\n\n")
	sb.WriteString("Email:\n")
	sb.WriteString(fmt.Sprintf("- From: %s\n", from))
	sb.WriteString(fmt.Sprintf("- Subject: %s\n", subject))
	if body != "" {
		sb.WriteString(fmt.Sprintf("- Body preview: %s\n", body))
	}
	sb.WriteString("\nAvailable labels:\n")
	for _, l := range labels {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", l.Name, l.Description))
	}
	sb.WriteString("\nRespond with JSON: {\"label\": \"<label_name>\"}")
	return sb.String()
}

func buildResponseSchema(labels []config.Label) map[string]any {
	labelNames := make([]any, len(labels))
	for i, l := range labels {
		labelNames[i] = l.Name
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"label": map[string]any{
				"type": "string",
				"enum": labelNames,
			},
		},
		"required":             []string{"label"},
		"additionalProperties": false,
	}
}
