package llm

import "encoding/base64"

// Role represents the sender of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// PartType indicates the content type of a message part.
type PartType string

const (
	PartTypeText  PartType = "text"
	PartTypeImage PartType = "image"
)

// Part is a single piece of content within a message.
// Construct with TextPart or ImagePart.
type Part struct {
	Type     PartType
	Text     string // valid when Type == PartTypeText
	MIMEType string // valid when Type == PartTypeImage (e.g. "image/png")
	Data     []byte // raw image bytes when Type == PartTypeImage
}

// TextPart returns a text Part.
func TextPart(text string) Part {
	return Part{Type: PartTypeText, Text: text}
}

// ImagePart returns an image Part with the given MIME type and raw bytes.
func ImagePart(mimeType string, data []byte) Part {
	return Part{Type: PartTypeImage, MIMEType: mimeType, Data: data}
}

// base64Encoded returns the base64 standard encoding of the image data.
func (p Part) base64Encoded() string {
	return base64.StdEncoding.EncodeToString(p.Data)
}

// Message is a single turn in the conversation.
type Message struct {
	Role  Role
	Parts []Part
}

// Request represents a prompt request sent to the LLM.
type Request struct {
	// System sets the system prompt. Optional.
	System string

	// Messages is the conversation. Typically one user message for single-turn
	// use, or multiple for multi-turn conversations.
	Messages []Message
}

// Response is the LLM's reply to a complete (non-streaming) request.
type Response struct {
	Text  string // generated text content
	Model string // model that produced the response
}
