package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func NewClient(cfg appconfig.LLMConfig) *Client {
	return &Client{
		apiKey:         strings.TrimSpace(cfg.APIKey),
		baseURL:        strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		model:          strings.TrimSpace(cfg.Model),
		embeddingModel: strings.TrimSpace(cfg.EmbeddingModel),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) EnhanceNote(ctx context.Context, input EnhanceNoteInput) (EnhancedNote, error) {
	if c.apiKey == "" {
		return EnhancedNote{}, fmt.Errorf("llm api key is empty, run config set --api-key")
	}
	if c.baseURL == "" {
		return EnhancedNote{}, fmt.Errorf("llm base url is empty, run config set --base-url")
	}
	if c.model == "" {
		return EnhancedNote{}, fmt.Errorf("llm model is empty, run config set --model")
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

	data, err := json.Marshal(reqBody)
	if err != nil {
		return EnhancedNote{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return EnhancedNote{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return EnhancedNote{}, err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return EnhancedNote{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return EnhancedNote{}, fmt.Errorf("llm request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(respData)))
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respData, &chatResp); err != nil {
		return EnhancedNote{}, err
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
	if c.apiKey == "" {
		return nil, fmt.Errorf("llm api key is empty, run config set --api-key")
	}
	if c.baseURL == "" {
		return nil, fmt.Errorf("llm base url is empty, run config set --base-url")
	}
	if c.embeddingModel == "" {
		return nil, fmt.Errorf("llm embedding model is empty, run config set --embedding-model")
	}

	reqBody := embeddingRequest{
		Model: c.embeddingModel,
		Input: text,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(respData)))
	}

	var embeddingResp embeddingResponse
	if err := json.Unmarshal(respData, &embeddingResp); err != nil {
		return nil, err
	}
	if len(embeddingResp.Data) == 0 {
		return nil, fmt.Errorf("embedding response has no data")
	}
	if len(embeddingResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding response vector is empty")
	}

	return embeddingResp.Data[0].Embedding, nil
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
