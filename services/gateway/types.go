package gateway

type chatCompletionRequest struct {
	Model               string         `json:"model"`
	Messages            []chatMessage  `json:"messages"`
	Stream              bool           `json:"stream"`
	StreamOptions       *streamOptions `json:"stream_options"`
	Temperature         *float32       `json:"temperature"`
	TopP                *float32       `json:"top_p"`
	MaxTokens           *int32         `json:"max_tokens"`
	MaxCompletionTokens *int32         `json:"max_completion_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type completionRequest struct {
	Model         string         `json:"model"`
	Prompt        any            `json:"prompt"`
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options"`
	Temperature   *float32       `json:"temperature"`
	TopP          *float32       `json:"top_p"`
	MaxTokens     *int32         `json:"max_tokens"`
}

type chatCompletionChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []chatChunkChoice `json:"choices"`
	Usage   *usage            `json:"usage,omitempty"`
}

type chatChunkChoice struct {
	Index        int       `json:"index"`
	Delta        chatDelta `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
}

type chatDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type chatCompletionResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []chatResponseChoice `json:"choices"`
	Usage   usage                `json:"usage"`
}

type chatResponseChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type textCompletionChunk struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []textCompletionChoice `json:"choices"`
	Usage   *usage                 `json:"usage,omitempty"`
}

type textCompletionChoice struct {
	Index        int     `json:"index"`
	Text         string  `json:"text"`
	FinishReason *string `json:"finish_reason"`
}

type textCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []textCompletionChoice `json:"choices"`
	Usage   usage                  `json:"usage"`
}

type usage struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
}

type modelsResponse struct {
	Object string        `json:"object"`
	Data   []modelObject `json:"data"`
}

type modelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
