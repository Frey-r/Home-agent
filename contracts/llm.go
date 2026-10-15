package contracts

import (
	"encoding/json"
	"fmt"
)

// Roles en el protocolo LLM v1.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ContentBlockType especifica el tipo de bloque de contenido polimórfico.
type ContentBlockType string

const (
	ContentTypeText       ContentBlockType = "text"
	ContentTypeImage      ContentBlockType = "image"
	ContentTypeToolCall   ContentBlockType = "tool_call"
	ContentTypeToolResult ContentBlockType = "tool_result"
)

// ContentBlock representa un fragmento polimórfico dentro de un mensaje.
type ContentBlock struct {
	Type ContentBlockType `json:"type"`

	// Para type == "text"
	Text string `json:"text,omitempty"`

	// Para type == "image"
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`

	// Para type == "tool_call"
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments any    `json:"arguments,omitempty"` // map[string]any o string JSON

	// Para type == "tool_result"
	ToolCallID string `json:"tool_call_id,omitempty"`
	Content    string `json:"content,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
}

// TextBlock crea un bloque de contenido textual.
func TextBlock(text string) ContentBlock {
	return ContentBlock{
		Type: ContentTypeText,
		Text: text,
	}
}

// ImageBlock crea un bloque de contenido de imagen codificada en base64.
func ImageBlock(mediaType, data string) ContentBlock {
	return ContentBlock{
		Type:      ContentTypeImage,
		MediaType: mediaType,
		Data:      data,
	}
}

// ToolCallBlock crea un bloque de llamada a herramienta generado por el LLM.
func ToolCallBlock(id, name string, args any) ContentBlock {
	return ContentBlock{
		Type:      ContentTypeToolCall,
		ID:        id,
		Name:      name,
		Arguments: args,
	}
}

// ToolResultBlock crea un bloque con el resultado de ejecución de una herramienta.
func ToolResultBlock(toolCallID, content string, isError bool) ContentBlock {
	return ContentBlock{
		Type:       ContentTypeToolResult,
		ToolCallID: toolCallID,
		Content:    content,
		IsError:    isError,
	}
}

// Message representa un turno dentro de la conversación.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// TextContent concatena todos los bloques de texto de un mensaje en un único string.
func (m Message) TextContent() string {
	var result string
	for _, b := range m.Content {
		if b.Type == ContentTypeText {
			result += b.Text
		}
	}
	return result
}

// ToolCalls devuelve todos los bloques de tipo tool_call presentes en el mensaje.
func (m Message) ToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, b := range m.Content {
		if b.Type == ContentTypeToolCall {
			calls = append(calls, b)
		}
	}
	return calls
}

// UserMessage crea un mensaje de usuario simple con un bloque de texto.
func UserMessage(text string) Message {
	return Message{
		Role:    RoleUser,
		Content: []ContentBlock{TextBlock(text)},
	}
}

// SystemMessage crea un mensaje de sistema simple con un bloque de texto.
func SystemMessage(text string) Message {
	return Message{
		Role:    RoleSystem,
		Content: []ContentBlock{TextBlock(text)},
	}
}

// AssistantMessage crea un mensaje del asistente simple con un bloque de texto.
func AssistantMessage(text string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: []ContentBlock{TextBlock(text)},
	}
}

// LLMTool define una herramienta expuesta al modelo de lenguaje.
type LLMTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// StopReason codifica la razón canónica de finalización del modelo.
type StopReason string

const (
	StopReasonEnd       StopReason = "end"
	StopReasonToolCalls StopReason = "tool_calls"
	StopReasonMaxTokens StopReason = "max_tokens"
	StopReasonRefusal   StopReason = "refusal"
	StopReasonError     StopReason = "error"
)

// LLMUsage contabiliza el consumo y cacheo de tokens.
type LLMUsage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// LLMRequestOptions especifica hiperparámetros de invocación.
type LLMRequestOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	ToolChoice  any      `json:"tool_choice,omitempty"`
	Stream      bool     `json:"-"`
}

// LLMRequest define una solicitud neutra completa para un LLM.
type LLMRequest struct {
	System   string            `json:"system,omitempty"`
	Messages []Message         `json:"messages"`
	Tools    []LLMTool         `json:"tools,omitempty"`
	Options  LLMRequestOptions `json:"options,omitempty"`
}

// LLMResponse define una respuesta neutra completa generada por un LLM.
type LLMResponse struct {
	Message    Message    `json:"message"`
	StopReason StopReason `json:"stop_reason"`
	Usage      LLMUsage   `json:"usage"`
}

// Text devuelve el contenido textual principal de la respuesta.
func (r LLMResponse) Text() string {
	return r.Message.TextContent()
}

// ToolCalls devuelve las llamadas a herramientas presentes en la respuesta.
func (r LLMResponse) ToolCalls() []ContentBlock {
	return r.Message.ToolCalls()
}

// StreamEventType representa el tipo de evento en streaming SSE.
type StreamEventType string

const (
	StreamEventTypeTextDelta     StreamEventType = "text_delta"
	StreamEventTypeToolCallDelta StreamEventType = "tool_call_delta"
	StreamEventTypeEnd           StreamEventType = "end"
)

// LLMStreamEvent encapsula un evento incremental durante una llamada con streaming.
type LLMStreamEvent struct {
	Type StreamEventType `json:"type"`

	// Para type == "text_delta"
	Text string `json:"text,omitempty"`

	// Para type == "tool_call_delta"
	Index          int    `json:"index,omitempty"`
	ToolCallID     string `json:"id,omitempty"`
	ToolName       string `json:"name,omitempty"`
	ArgumentsDelta string `json:"arguments_delta,omitempty"`

	// Para type == "end"
	StopReason StopReason `json:"stop_reason,omitempty"`
	Usage      LLMUsage   `json:"usage,omitempty"`
}

// MarshalArgumentsHelper asegura que los argumentos de una llamada a herramienta se obtengan como string JSON.
func (b ContentBlock) ArgumentsJSON() (string, error) {
	if b.Type != ContentTypeToolCall {
		return "", fmt.Errorf("el bloque no es de tipo tool_call: %s", b.Type)
	}
	switch v := b.Arguments.(type) {
	case string:
		return v, nil
	case nil:
		return "{}", nil
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("error serializando arguments: %w", err)
		}
		return string(raw), nil
	}
}
