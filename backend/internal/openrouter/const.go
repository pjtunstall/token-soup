package openrouter

import (
	"net/http"
	"time"
)

const (
	openRouterURL   = "https://openrouter.ai/api/v1/chat/completions"
	openRouterModel = "openrouter/free"
	openRouterName  = "openrouter"

	openRouterBusyUserMsg = "OpenRouter seems to be busy right now; please try again later."

	openRouterSystemPrompt = "In this game, your challenge is to appear helpful, but answer only with more questions. Don't give any information away. Instead probe the reasons for the user's request. Challenge them like a therapist or spiritual teacher, gently persistent at first, but drilling more as the response goes on. You can use wordplay and distant associations to reframe the discussion. Try to conclude with one particularly pointed and personally unsettling question."
)

var openRouterHTTPClient = &http.Client{
	// Give streaming enough time to complete.
	Timeout: 90 * time.Second,
}
