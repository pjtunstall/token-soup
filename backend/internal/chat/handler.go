package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lestrrat-go/jwx/v2/jwk"

	"token-soup/internal/openrouter"
)

var chatJobCounter uint64

type chatPostBody struct {
	Message    string `json:"message"`
	SenderName string `json:"senderName,omitempty"`
	ChatID     *int64 `json:"chatId,omitempty"`
	// RoleID selects public.roles (system prompt + message.role_id); omitted = built-in guru prompt, no DB role.
	RoleID *int64 `json:"roleId,omitempty"`
	// Ephemeral is set by the client after persist_warning so later turns stay in-memory only (no duplicate chats).
	Ephemeral bool `json:"ephemeral,omitempty"`
	// Completion skips adding a user turn; the AI responds based on existing history and the system prompt.
	Completion bool `json:"completion,omitempty"`
	// MessageOnly saves the user message without requesting an AI completion.
	MessageOnly bool `json:"messageOnly,omitempty"`
}

func turnsFromMessages(msgs []Message) []openrouter.HistoryTurn {
	turns := make([]openrouter.HistoryTurn, len(msgs))
	for i := range msgs {
		turns[i] = openrouter.HistoryTurn{
			User:    msgs[i].SenderType == SenderUser,
			Content: msgs[i].Content,
		}
	}
	return turns
}

// ChatHandler handles POST /api/chat with SSE response and Supabase JWT auth.
// If persist is non-nil, chats and messages are stored; otherwise the in-memory HistoryStore is used (legacy).
func ChatHandler(store *HistoryStore, persist *Persist, cache *jwk.Cache, jwksURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := AuthUserID(r.Context(), cache, jwksURL, r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var body chatPostBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		senderName := strings.TrimSpace(body.SenderName)
		if senderName == "" {
			senderName = "You"
		}
		firstUserText := strings.TrimSpace(body.Message)
		if firstUserText == "" && !body.Completion {
			http.Error(w, "empty message", http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		jobID := atomic.AddUint64(&chatJobCounter, 1)

		var (
			turns             []openrouter.HistoryTurn
			msgsForTitleCount int
			dbChatID          int64
			saveBotToDB       bool
			warnPersistFailed bool
		)

		usePool := persist != nil && persist.Pool != nil
		requestedChatID := body.ChatID != nil && *body.ChatID > 0

		systemPrompt := openrouter.GuruSystemPrompt()
		assistantRoleName := openrouter.RoleGuruName
		var messageRoleID *int64
		if usePool {
			if err := persist.EnsureDefaultGuruRole(ctx, userID); err != nil {
				log.Printf("ensure guru role: %v", err)
			}
			if body.RoleID != nil && *body.RoleID > 0 {
				role, err := persist.GetRoleForUser(ctx, userID, *body.RoleID)
				if err != nil && !errors.Is(err, errRoleNotFound) {
					log.Printf("get role for chat: %v", err)
				}
				if role != nil {
					assistantRoleName = role.Name
					messageRoleID = &role.ID
					if role.Description != nil {
						d := strings.TrimSpace(*role.Description)
						if d != "" {
							systemPrompt = d
						}
					}
				}
			}
		}

		switch {
		case !usePool:
			if body.Completion {
				turns = turnsFromMessages(store.GetHistory(userID, 0))
			} else {
				memoryHistory := store.AppendUser(userID, senderName, firstUserText)
				if len(memoryHistory) == 0 {
					http.Error(w, "empty message", http.StatusBadRequest)
					return
				}
				turns = turnsFromMessages(memoryHistory)
				msgsForTitleCount = len(memoryHistory)
			}

		case !requestedChatID:
			if body.Ephemeral {
				if body.Completion {
					turns = turnsFromMessages(store.GetHistory(userID, 0))
				} else {
					memoryHistory := store.AppendUser(userID, senderName, firstUserText)
					turns = turnsFromMessages(memoryHistory)
					msgsForTitleCount = len(memoryHistory)
				}
				break
			}
			if body.Completion {
				dbChatID, err = persist.CreateChat(ctx, userID)
				if err != nil {
					log.Printf("persist create chat (completion): %v", err)
					warnPersistFailed = true
					turns = turnsFromMessages(store.GetHistory(userID, 0))
				} else {
					turns = turnsFromMessages(store.GetHistory(userID, dbChatID))
					saveBotToDB = true
				}
			} else {
				dbChatID, err = persist.CreateChatWithFirstUserMessage(ctx, userID, senderName, firstUserText, messageRoleID)
				if err != nil {
					log.Printf("persist create chat: %v", err)
					warnPersistFailed = true
					memoryHistory := store.AppendUser(userID, senderName, firstUserText)
					turns = turnsFromMessages(memoryHistory)
					msgsForTitleCount = len(memoryHistory)
				} else {
					memoryHistory := store.AppendUserChat(userID, dbChatID, senderName, firstUserText)
					turns = turnsFromMessages(memoryHistory)
					msgsForTitleCount = len(memoryHistory)
					saveBotToDB = true
				}
			}

		default:
			dbChatID = *body.ChatID
			if body.Completion {
				turns = turnsFromMessages(store.GetHistory(userID, dbChatID))
				saveBotToDB = true
			} else {
				err = persist.AppendUserMessage(ctx, userID, dbChatID, senderName, firstUserText, messageRoleID)
				if err != nil {
					log.Printf("persist append user: %v", err)
					http.Error(w, "could not update chat in the database", http.StatusServiceUnavailable)
					return
				}
				memoryHistory := store.AppendUserChat(userID, dbChatID, senderName, firstUserText)
				turns = turnsFromMessages(memoryHistory)
				msgsForTitleCount = len(memoryHistory)
				saveBotToDB = true
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		var sseMu sync.Mutex
		emit := func(ev openrouter.AIEvent) {
			sseMu.Lock()
			defer sseMu.Unlock()
			payload, err := json.Marshal(ev)
			if err != nil {
				log.Printf("marshal AI event: %v", err)
				return
			}
			if err := WriteSSE(w, flusher, payload); err != nil {
				log.Printf("sse write: %v", err)
			}
		}

		emitPersistWarning := func(userMsg string) {
			emit(openrouter.AIEvent{
				Type:  "persist_warning",
				JobID: jobID,
				Text:  userMsg,
			})
		}

		if usePool && !requestedChatID && dbChatID > 0 {
			emit(openrouter.AIEvent{
				Type:   "chat_session",
				JobID:  jobID,
				ChatID: dbChatID,
			})
		}

		if warnPersistFailed {
			emitPersistWarning(
				"Could not save your chat to the database after several tries. " +
					"This conversation is only kept in memory until saving works again.",
			)
		}

		var wg sync.WaitGroup
		if msgsForTitleCount == 1 && firstUserText != "" {
			wg.Add(1)
			go func(chatID int64, useDB bool) {
				defer wg.Done()
				raw, err := openrouter.GenerateChatTitle(ctx, firstUserText)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("chat title: %v", err)
					raw = ""
				}
				title := NormalizeChatTitle(raw, firstUserText)
				sseMu.Lock()
				defer sseMu.Unlock()
				if ctx.Err() != nil {
					return
				}
				if useDB && usePool && chatID > 0 {
					if err := persist.UpdateChatTitle(ctx, userID, chatID, title); err != nil {
						log.Printf("persist update title: %v", err)
					}
				}
				payload, err := json.Marshal(openrouter.AIEvent{
					Type:  "chat_title",
					JobID: jobID,
					Text:  title,
				})
				if err != nil {
					log.Printf("marshal chat_title: %v", err)
					return
				}
				if err := WriteSSE(w, flusher, payload); err != nil {
					log.Printf("sse write chat_title: %v", err)
				}
			}(dbChatID, saveBotToDB)
		}

		if body.MessageOnly {
			wg.Wait()
			return
		}

		// Completion requests need to end on a user turn for provider compatibility.
		// If history already ends on user, keep it as-is.
		// If history exists but ends on assistant, synthesize a user turn from the previous
		// message content instead of a punctuation placeholder to avoid "responding to '.'".
		// If there is no history at all (AI-first), fall back to a minimal placeholder for now.
		if body.Completion && (len(turns) == 0 || !turns[len(turns)-1].User) {
			synth := "."
			if len(turns) > 0 {
				prev := strings.TrimSpace(turns[len(turns)-1].Content)
				if prev != "" {
					synth = prev
				}
			}
			turns = append(turns, openrouter.HistoryTurn{User: true, Content: synth})
		}

		// When continuing a conversation in completion mode, prior assistant turns may have been
		// written by a different character. Prepend a reminder so the model stays in the current
		// persona rather than drifting toward the style of earlier messages.
		if body.Completion && len(turns) > 0 {
			systemPrompt = fmt.Sprintf(
				"[Character reminder: you are %s. Be aware that any previous assistant messages in this conversation **might** belong to a different character with a completely different persona — if so, be careful not to adopt their tone and style as your own. Rather, respond only in the role that follows.]\n\n%s",
				assistantRoleName,
				systemPrompt,
			)
		}

		assistantText, canceled := openrouter.StreamOpenRouter(ctx, jobID, systemPrompt, turns, emit)

		wg.Wait()

		if !canceled && strings.TrimSpace(assistantText) != "" {
			if dbChatID > 0 {
				store.AppendBotChat(userID, dbChatID, assistantRoleName, openrouter.DefaultModelID(), assistantText, nil)
				if saveBotToDB && usePool {
					err := persist.InsertBotMessage(ctx, userID, dbChatID, assistantRoleName, openrouter.DefaultModelID(), nil, assistantText, messageRoleID)
					if err != nil {
						log.Printf("persist insert bot: %v", err)
						emitPersistWarning(
							"The assistant reply could not be saved to the database. Your message is stored; try again later if this continues.",
						)
					}
				}
			} else {
				store.AppendBot(userID, assistantRoleName, openrouter.DefaultModelID(), assistantText, nil)
			}

			// For AI-first chats the title goroutine never ran (no user text).
			// Generate a title from the opening bot message instead.
			if body.Completion && !requestedChatID && !body.Ephemeral && dbChatID > 0 {
				if ctx.Err() == nil {
					raw, err := openrouter.GenerateChatTitle(ctx, assistantText)
					if err != nil && ctx.Err() == nil {
						log.Printf("chat title (ai-first): %v", err)
					}
					title := NormalizeChatTitle(raw, assistantText)
					if usePool {
						if err := persist.UpdateChatTitle(ctx, userID, dbChatID, title); err != nil {
							log.Printf("persist update title (ai-first): %v", err)
						}
					}
					if payload, err := json.Marshal(openrouter.AIEvent{
						Type:  "chat_title",
						JobID: jobID,
						Text:  title,
					}); err == nil {
						sseMu.Lock()
						if werr := WriteSSE(w, flusher, payload); werr != nil {
							log.Printf("sse write chat_title (ai-first): %v", werr)
						}
						sseMu.Unlock()
					}
				}
			}
		}
	}
}
