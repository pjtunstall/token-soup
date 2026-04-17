package openrouter

// AIEvent is one JSON object sent on the chat SSE stream (same shape as the old WebSocket payloads).
// type "chat_title" carries a session title in Text (first turn only); Name is omitted.
type AIEvent struct {
	Type  string `json:"type"`
	JobID uint64 `json:"jobId"`
	Text  string `json:"text,omitempty"`
	// Name is the assistant label (from OpenRouter’s resolved model id), without trailing colon.
	Name string `json:"name,omitempty"`
	// ChatID is set for type "chat_session" after the chat row is stored (first message).
	ChatID int64 `json:"chatId,omitempty"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// HistoryTurn is one user or assistant message for the OpenRouter chat API (bot → assistant).
type HistoryTurn struct {
	User    bool
	Content string
}

type openRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Stream      bool                `json:"stream,omitempty"`
	Temperature float32             `json:"temperature,omitempty"`
}

type openRouterChoice struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
}

type openRouterError struct {
	Message string `json:"message"`
}

type openRouterResponse struct {
	Choices []openRouterChoice `json:"choices"`
	Error   openRouterError    `json:"error"`
}

type openRouterStreamDelta struct {
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	Reasoning        string `json:"reasoning,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type openRouterStreamChoice struct {
	Delta openRouterStreamDelta `json:"delta"`
}

type openRouterStreamResponse struct {
	Model   string                   `json:"model,omitempty"`
	Choices []openRouterStreamChoice `json:"choices"`
	Error   openRouterError          `json:"error"`
	Usage   openRouterUsage          `json:"usage"`
}

type openRouterUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

func (u openRouterUsage) hasAny() bool {
	return u.PromptTokens > 0 || u.CompletionTokens > 0 || u.TotalTokens > 0
}
