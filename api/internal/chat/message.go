package chat

// SenderType matches public.messages.sender_type.
type SenderType string

const (
	SenderUser SenderType = "user"
	SenderBot  SenderType = "bot"
)

// Message mirrors public.messages columns used in memory before persistence.
// Omits id, chat_id, and created_at until loaded from the database.
type Message struct {
	RoleID *int64
	// SenderName is the human-facing label (user display name or role/persona name for the bot).
	SenderName string
	SenderType SenderType
	// SenderModel is required for bot messages (OpenRouter model id); nil for user.
	SenderModel *string
	// Reasoning is optional; only meaningful for bot messages.
	Reasoning *string
	Content string
}
