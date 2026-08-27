package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	appconfig "ai-dev-logger/internal/config"
)

type Client struct {
	apiKey         string
	baseURL        string
	model          string
	embeddingModel string
	httpClient     *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
	maxRetryDelay  time.Duration
}

type EnhanceNoteInput struct {
	Title string
	Body  string
	Tags  []string
}

type EnhancedNote struct {
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags"`
}

// SearchNote is the small amount of note context needed for a search explanation.
type SearchNote struct {
	ID      int64
	Score   float64
	Title   string
	Tags    []string
	Summary string
	Body    string
}

func NewClient(cfg appconfig.LLMConfig) *Client {
	apiKey, _ := appconfig.ResolveAPIKey(cfg.APIKey)
	return &Client{
		apiKey:         apiKey,
		baseURL:        strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		model:          strings.TrimSpace(cfg.Model),
		embeddingModel: strings.TrimSpace(cfg.EmbeddingModel),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		maxRetries:     defaultMaxRetries,
		retryBaseDelay: defaultRetryBaseDelay,
		maxRetryDelay:  defaultMaxRetryDelay,
	}
}

func (c *Client) EnhanceNote(ctx context.Context, input EnhanceNoteInput) (EnhancedNote, error) {
	if err := c.ValidateChatConfig(); err != nil {
		return EnhancedNote{}, err
	}

	reqBody := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: enhanceSystemPrompt,
			},
			{
				Role:    "user",
				Content: buildEnhanceUserPrompt(input),
			},
		},
		Temperature:    0.2,
		ResponseFormat: map[string]string{"type": "json_object"},
	}

	var chatResp chatCompletionResponse
	if err := c.postJSON(ctx, "/chat/completions", reqBody, &chatResp); err != nil {
		return EnhancedNote{}, fmt.Errorf("enhance note: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return EnhancedNote{}, fmt.Errorf("llm response has no choices")
	}

	return parseEnhancedNote(chatResp.Choices[0].Message.Content)
}

func (c *Client) CreateEmbedding(ctx context.Context, text string) ([]float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("embedding input is empty")
	}
	if err := c.ValidateEmbeddingConfig(); err != nil {
		return nil, err
	}

	reqBody := embeddingRequest{
		Model: c.embeddingModel,
		Input: text,
	}

	var embeddingResp embeddingResponse
	if err := c.postJSON(ctx, "/embeddings", reqBody, &embeddingResp); err != nil {
		return nil, fmt.Errorf("create embedding: %w", err)
	}
	if len(embeddingResp.Data) == 0 {
		return nil, fmt.Errorf("embedding response has no data")
	}
	if len(embeddingResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding response vector is empty")
	}

	return embeddingResp.Data[0].Embedding, nil
}

// ValidateEmbeddingConfig checks batch-wide settings before API work begins.
func (c *Client) ValidateEmbeddingConfig() error {
	if c.apiKey == "" {
		return missingAPIKeyError()
	}
	if c.baseURL == "" {
		return fmt.Errorf("llm base url is empty, run config set --base-url")
	}
	if c.embeddingModel == "" {
		return fmt.Errorf("llm embedding model is empty, run config set --embedding-model")
	}
	return nil
}

// ValidateChatConfig checks settings shared by chat-based operations.
func (c *Client) ValidateChatConfig() error {
	if c.apiKey == "" {
		return missingAPIKeyError()
	}
	if c.baseURL == "" {
		return fmt.Errorf("llm base url is empty, run config set --base-url")
	}
	if c.model == "" {
		return fmt.Errorf("llm model is empty, run config set --model")
	}
	return nil
}

// ProbeChat sends a minimal request without creating or changing a note.
func (c *Client) ProbeChat(ctx context.Context) error {
	if err := c.ValidateChatConfig(); err != nil {
		return err
	}

	reqBody := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: "This is a connection check. Reply with OK."},
			{Role: "user", Content: "OK"},
		},
		Temperature: 0,
	}

	var chatResp chatCompletionResponse
	if err := c.postJSON(ctx, "/chat/completions", reqBody, &chatResp); err != nil {
		return fmt.Errorf("probe chat: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return fmt.Errorf("probe chat: llm response has no choices")
	}
	return nil
}

// ProbeEmbedding sends a small input and returns the vector dimensions.
func (c *Client) ProbeEmbedding(ctx context.Context) (int, error) {
	embedding, err := c.CreateEmbedding(ctx, "ai-dev-logger connection check")
	if err != nil {
		return 0, fmt.Errorf("probe embedding: %w", err)
	}
	return len(embedding), nil
}

// ExplainSearch explains how the retrieved notes relate to a user's question.
func (c *Client) ExplainSearch(ctx context.Context, query string, notes []SearchNote) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("search query is empty")
	}
	if len(notes) == 0 {
		return "", fmt.Errorf("search notes are empty")
	}
	if err := c.ValidateChatConfig(); err != nil {
		return "", err
	}

	reqBody := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: searchExplainSystemPrompt},
			{Role: "user", Content: buildSearchExplainPrompt(query, notes)},
		},
		Temperature: 0.2,
	}
	var chatResp chatCompletionResponse
	if err := c.postJSON(ctx, "/chat/completions", reqBody, &chatResp); err != nil {
		return "", fmt.Errorf("explain search: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm response has no choices")
	}

	explanation := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if explanation == "" {
		return "", fmt.Errorf("llm explanation is empty")
	}
	return explanation, nil
}

func missingAPIKeyError() error {
	return fmt.Errorf("llm api key is empty, set %s or run config set --api-key", appconfig.EnvAPIKey)
}

type chatCompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

const enhanceSystemPrompt = `You are an assistant that cleans and structures programming notes.
Return only a JSON object with these fields:
- title: concise note title
- body: polished Markdown body, preserving code blocks
- summary: one short Chinese summary
- tags: 3 to 6 lowercase tags`

const searchExplainSystemPrompt = `You are a programming knowledge assistant.
Answer the user's question using only the retrieved local notes.
Write concise Chinese Markdown. State uncertainty when the notes do not fully answer the question.
For every factual claim, cite supporting notes using [Note #ID].
Do not invent APIs, code details, or facts not present in the notes.`

func buildEnhanceUserPrompt(input EnhanceNoteInput) string {
	tagsJSON, _ := json.Marshal(input.Tags)

	return fmt.Sprintf(`Please improve this programming note.

Original title:
%s

Original tags:
%s

Original body:
%s

Return JSON only.`, input.Title, string(tagsJSON), input.Body)
}

func buildSearchExplainPrompt(query string, notes []SearchNote) string {
	var builder strings.Builder
	builder.WriteString("User question:\n")
	builder.WriteString(query)
	builder.WriteString("\n\nRetrieved notes:\n")

	for _, note := range notes {
		fmt.Fprintf(&builder, "\n[Note #%d]\n", note.ID)
		fmt.Fprintf(&builder, "Similarity: %.4f\n", note.Score)
		fmt.Fprintf(&builder, "Title: %s\n", note.Title)
		if len(note.Tags) > 0 {
			fmt.Fprintf(&builder, "Tags: %s\n", strings.Join(note.Tags, ", "))
		}
		if strings.TrimSpace(note.Summary) != "" {
			fmt.Fprintf(&builder, "Summary: %s\n", note.Summary)
		}
		fmt.Fprintf(&builder, "Body: %s\n", truncateRunes(note.Body, 1200))
	}
	return builder.String()
}

func truncateRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func parseEnhancedNote(content string) (EnhancedNote, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	var enhanced EnhancedNote
	if err := json.Unmarshal([]byte(content), &enhanced); err != nil {
		return EnhancedNote{}, fmt.Errorf("parse llm JSON response: %w", err)
	}

	enhanced.Title = strings.TrimSpace(enhanced.Title)
	enhanced.Body = strings.TrimSpace(enhanced.Body)
	enhanced.Summary = strings.TrimSpace(enhanced.Summary)
	enhanced.Tags = cleanTags(enhanced.Tags)

	return enhanced, nil
}

func cleanTags(tags []string) []string {
	seen := map[string]struct{}{}
	cleaned := make([]string, 0, len(tags))

	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, key)
	}

	return cleaned
}
