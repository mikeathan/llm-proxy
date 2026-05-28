package proxy

import "llm-proxy/models"

type ChatRole = models.ChatRole
type ToolChoice = models.ToolChoice

const (
	SystemRole    = models.SystemRole
	UserRole      = models.UserRole
	AssistantRole = models.AssistantRole
	ToolRole      = models.ToolRole

	ToolChoiceAuto     = models.ToolChoiceAuto
	ToolChoiceRequired = models.ToolChoiceRequired
	ToolChoiceNone     = models.ToolChoiceNone
)

type ExecutionHistory = models.ExecutionHistory

// Message
type Message = models.Message

// Chat Request
type ChatRequest = models.ChatRequest

type ToolCall = models.ToolCall
type FunctionCall = models.FunctionCall

type Choice = models.Choice

// Chat Response
type ChatResponse = models.ChatResponse

type Tool = models.Tool
type FunctionSchema = models.FunctionSchema
type ResponseFormat = models.ResponseFormat
