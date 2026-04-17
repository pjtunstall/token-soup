package openrouter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func parseSSEStream(resp *http.Response, onData func(data []byte) error) error {
	// OpenRouter streams SSE. We parse `data:` lines and treat a blank line as
	// the end of one SSE event.
	reader := bufio.NewReader(resp.Body)
	var dataLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if len(dataLines) > 0 {
				joined := strings.Join(dataLines, "\n")
				if err := onData([]byte(joined)); err != nil {
					return err
				}
				dataLines = dataLines[:0]
			}
			if err == io.EOF {
				return nil
			}
			continue
		}

		// Ignore SSE comments (lines starting with ':').
		if strings.HasPrefix(line, ":") {
			if err == io.EOF {
				return nil
			}
			continue
		}

		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			val := strings.TrimSpace(rest)
			dataLines = append(dataLines, val)
		}

		if err == io.EOF {
			// If the stream ended without a blank line, flush what we have.
			if len(dataLines) > 0 {
				joined := strings.Join(dataLines, "\n")
				return onData([]byte(joined))
			}
			return nil
		}
	}
}

// openRouterStreamState holds mutable state while parsing the SSE stream.
type openRouterStreamState struct {
	ctx     context.Context
	jobID   uint64
	emit    func(AIEvent)
	started time.Time

	assistantName string

	buf                    strings.Builder
	aiBubbleStarted        bool
	sentFirstNonWhitespace bool
	firstDeltaLogged       bool
	latestUsage            openRouterUsage
	logDeltaStream         bool
	logDeltaStreamStarted  bool
}

func (s *openRouterStreamState) endDeltaStreamLogLine() {
	if s.logDeltaStreamStarted {
		_, _ = fmt.Fprint(os.Stdout, "\n")
	}
}

func (s *openRouterStreamState) ensureDeltaStreamPrefix() {
	if !s.logDeltaStreamStarted {
		s.logDeltaStreamStarted = true
		_, _ = fmt.Fprintf(os.Stdout, "openrouter: job=%d delta_stream=", s.jobID)
	}
}

// startAIBubble emits ai_start once when the assistant message bubble begins.
func (s *openRouterStreamState) startAIBubble() {
	if s.aiBubbleStarted {
		return
	}
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_start",
		Text: s.assistantName + ": ",
	})
	s.aiBubbleStarted = true
}

// maybeStartBubbleForChunk starts the bubble when this SSE chunk carries model info or choices,
// so the UI can show the header before the first token.
func (s *openRouterStreamState) maybeStartBubbleForChunk(chunk *openRouterStreamResponse) {
	if len(chunk.Choices) == 0 && strings.TrimSpace(chunk.Model) == "" {
		return
	}
	s.startAIBubble()
}

func (s *openRouterStreamState) applyChunkMetadata(chunk *openRouterStreamResponse) {
	if chunk.Model != "" {
		s.assistantName = formatModelDisplayName(chunk.Model)
	}
	if chunk.Usage.hasAny() {
		s.latestUsage = chunk.Usage
	}
}

func (s *openRouterStreamState) unmarshalSSEData(data []byte) (openRouterStreamResponse, error) {
	var chunk openRouterStreamResponse
	if err := json.Unmarshal(data, &chunk); err != nil {
		emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
			Type: "ai_replace",
			Text: s.assistantName + ": [invalid response]",
		})
		return chunk, err
	}
	return chunk, nil
}

func (s *openRouterStreamState) emitReasoningDelta(reasoningPart string) {
	if reasoningPart == "" {
		return
	}
	if s.logDeltaStream {
		s.ensureDeltaStreamPrefix()
		_, _ = fmt.Fprint(os.Stdout, reasoningPart)
	} else {
		log.Printf("openrouter: job=%d reasoning_delta=%q", s.jobID, strings.TrimSpace(reasoningPart))
	}
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_reasoning_delta",
		Text: reasoningPart,
	})
}

// processContentDelta handles one content delta: leading-whitespace trim for the first visible
// token, optional delta logging, buffer append, and ai_delta emission.
func (s *openRouterStreamState) processContentDelta(deltaText string) {
	if deltaText == "" {
		return
	}
	s.startAIBubble()
	if !s.sentFirstNonWhitespace {
		deltaText = strings.TrimLeft(deltaText, " \n\t\r")
		if deltaText == "" {
			return
		}
		s.sentFirstNonWhitespace = true
	}
	if !s.firstDeltaLogged {
		s.firstDeltaLogged = true
		log.Printf("openrouter: job=%d first_delta_after=%s", s.jobID, time.Since(s.started).Truncate(time.Millisecond))
	}
	if s.logDeltaStream {
		s.ensureDeltaStreamPrefix()
		_, _ = fmt.Fprint(os.Stdout, deltaText)
	}
	s.buf.WriteString(deltaText)
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_delta",
		Text: deltaText,
	})
}

func (s *openRouterStreamState) onSSEData(data []byte) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[DONE]" {
		return nil
	}

	chunk, err := s.unmarshalSSEData(data)
	if err != nil {
		return err
	}
	s.applyChunkMetadata(&chunk)
	s.maybeStartBubbleForChunk(&chunk)

	for _, choice := range chunk.Choices {
		reasoningPart := choice.Delta.Reasoning
		if reasoningPart == "" {
			reasoningPart = choice.Delta.ReasoningContent
		}
		s.emitReasoningDelta(reasoningPart)
		s.processContentDelta(choice.Delta.Content)
	}
	return nil
}

func handleOpenRouterStreamError(
	s *openRouterStreamState,
	streamErr error,
) (canceled bool) {
	if errors.Is(streamErr, context.Canceled) || errors.Is(s.ctx.Err(), context.Canceled) {
		log.Printf("openrouter: job=%d canceled during stream after=%s", s.jobID, time.Since(s.started).Truncate(time.Millisecond))
		s.endDeltaStreamLogLine()
		return true
	}
	log.Printf("openrouter: job=%d stream error after=%s: %v", s.jobID, time.Since(s.started).Truncate(time.Millisecond), streamErr)
	s.endDeltaStreamLogLine()
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_replace",
		Text: s.assistantName + ": [stream error]",
	})
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_end",
	})
	return false
}
