package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"token-soup/internal/openrouter"
)

// errChatNotFound is returned by GetChatTitle when the chat id is missing or not owned by the user.
var errChatNotFound = errors.New("chat not found")

// errRoleNotFound is returned by GetRoleForUser when the role id is missing or not owned by the user.
var errRoleNotFound = errors.New("role not found")

func sqlNullInt64(n *int64) interface{} {
	if n == nil {
		return nil
	}
	return *n
}

// Persist performs chat/message writes against Postgres (service role or any role with table access).
type Persist struct {
	Pool *pgxpool.Pool
}

// EnsureDefaultGuruRole inserts the canonical "Guru" template row if the user has none yet (idempotent).
func (p *Persist) EnsureDefaultGuruRole(ctx context.Context, userID string) error {
	desc := openrouter.GuruSystemPrompt()
	return withPersistRetry(ctx, func() error {
		_, err := p.Pool.Exec(ctx, `
			INSERT INTO public.roles (user_id, name, description, created_at, last_updated)
			VALUES ($1::uuid, 'Guru', $2, now(), now())
			ON CONFLICT ON CONSTRAINT roles_user_id_name_unique DO NOTHING`,
			userID, desc,
		)
		return err
	})
}

// GetRoleForUser returns a role by id if it belongs to userID.
func (p *Persist) GetRoleForUser(ctx context.Context, userID string, roleID int64) (*Role, error) {
	var r Role
	var desc pgtype.Text
	err := withPersistRetry(ctx, func() error {
		qerr := p.Pool.QueryRow(ctx, `
			SELECT id, name, description
			FROM public.roles
			WHERE id = $1 AND user_id = $2::uuid`,
			roleID, userID,
		).Scan(&r.ID, &r.Name, &desc)
		if errors.Is(qerr, pgx.ErrNoRows) {
			return errRoleNotFound
		}
		if qerr != nil {
			return qerr
		}
		if desc.Valid {
			s := desc.String
			r.Description = &s
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errRoleNotFound) {
			return nil, errRoleNotFound
		}
		return nil, err
	}
	return &r, nil
}

// CreateChatWithFirstUserMessage inserts a row in chats and the first user message in one transaction.
func (p *Persist) CreateChatWithFirstUserMessage(ctx context.Context, userID, senderName, content string, roleID *int64) (chatID int64, err error) {
	err = withPersistRetry(ctx, func() error {
		tx, err := p.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)

		err = tx.QueryRow(ctx, `
			INSERT INTO public.chats (user_id, title)
			VALUES ($1::uuid, 'New Chat')
			RETURNING id`,
			userID,
		).Scan(&chatID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO public.messages (chat_id, role_id, sender_name, sender_type, sender_model, reasoning, content)
			VALUES ($1, $2, $3, 'user', NULL, NULL, $4)`,
			chatID, sqlNullInt64(roleID), senderName, content,
		)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	return chatID, err
}

// AppendUserMessage inserts a user message if the chat exists and belongs to userID.
func (p *Persist) AppendUserMessage(ctx context.Context, userID string, chatID int64, senderName, content string, roleID *int64) error {
	return withPersistRetry(ctx, func() error {
		tag, err := p.Pool.Exec(ctx, `
			INSERT INTO public.messages (chat_id, role_id, sender_name, sender_type, sender_model, reasoning, content)
			SELECT c.id, $5, $2, 'user', NULL, NULL, $3
			FROM public.chats c
			WHERE c.id = $1 AND c.user_id = $4::uuid`,
			chatID, senderName, content, userID, sqlNullInt64(roleID),
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("chat not found or access denied")
		}
		return nil
	})
}

// ChatSummary is one row from public.chats for GET /api/chats.
type ChatSummary struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	LastUpdated time.Time `json:"lastUpdated"`
}

// ListChats returns the user’s chats, most recently updated first.
func (p *Persist) ListChats(ctx context.Context, userID string) ([]ChatSummary, error) {
	var out []ChatSummary
	err := withPersistRetry(ctx, func() error {
		rows, err := p.Pool.Query(ctx, `
			SELECT id, title, last_updated
			FROM public.chats
			WHERE user_id = $1::uuid
			ORDER BY last_updated DESC`,
			userID,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		out = nil
		for rows.Next() {
			var s ChatSummary
			if err := rows.Scan(&s.ID, &s.Title, &s.LastUpdated); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}

// GetChatTitle returns the title if the chat exists and belongs to userID.
func (p *Persist) GetChatTitle(ctx context.Context, userID string, chatID int64) (string, error) {
	var title string
	err := withPersistRetry(ctx, func() error {
		qerr := p.Pool.QueryRow(ctx, `
			SELECT title FROM public.chats
			WHERE id = $1 AND user_id = $2::uuid`,
			chatID, userID,
		).Scan(&title)
		if errors.Is(qerr, pgx.ErrNoRows) {
			return errChatNotFound
		}
		return qerr
	})
	return title, err
}

// LoadMessages returns all messages for a chat owned by userID, ordered by id.
// Caller should ensure the chat exists (e.g. GetChatTitle) if a 404 distinction is required.
func (p *Persist) LoadMessages(ctx context.Context, userID string, chatID int64) ([]Message, error) {
	var rows pgx.Rows
	err := withPersistRetry(ctx, func() error {
		var qerr error
		rows, qerr = p.Pool.Query(ctx, `
			SELECT m.role_id, m.sender_name, m.sender_type, m.sender_model, m.reasoning, m.content
			FROM public.messages m
			INNER JOIN public.chats c ON c.id = m.chat_id
			WHERE m.chat_id = $1 AND c.user_id = $2::uuid
			ORDER BY m.id ASC`,
			chatID, userID,
		)
		return qerr
	})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var model *string
		var roleID pgtype.Int8
		if err := rows.Scan(&roleID, &m.SenderName, &m.SenderType, &model, &m.Reasoning, &m.Content); err != nil {
			return nil, err
		}
		m.SenderModel = model
		if roleID.Valid {
			v := roleID.Int64
			m.RoleID = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// InsertBotMessage appends an assistant turn for the chat (must already exist and belong to user).
func (p *Persist) InsertBotMessage(ctx context.Context, userID string, chatID int64, senderName, model string, reasoning *string, content string, roleID *int64) error {
	return withPersistRetry(ctx, func() error {
		tag, err := p.Pool.Exec(ctx, `
			INSERT INTO public.messages (chat_id, role_id, sender_name, sender_type, sender_model, reasoning, content)
			SELECT c.id, $7, $2, 'bot', $3, $4, $5
			FROM public.chats c
			WHERE c.id = $1 AND c.user_id = $6::uuid`,
			chatID, senderName, model, reasoning, content, userID, sqlNullInt64(roleID),
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("chat not found or access denied")
		}
		return nil
	})
}

// UpdateChatTitle sets the chat title after generation (best-effort).
func (p *Persist) UpdateChatTitle(ctx context.Context, userID string, chatID int64, title string) error {
	return withPersistRetry(ctx, func() error {
		tag, err := p.Pool.Exec(ctx, `
			UPDATE public.chats
			SET title = $1, last_updated = now()
			WHERE id = $2 AND user_id = $3::uuid`,
			title, chatID, userID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("chat not found or access denied")
		}
		return nil
	})
}

// Role is a subset of public.roles for API responses.
type Role struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// ListRoles returns all roles for the user, ordered by name.
func (p *Persist) ListRoles(ctx context.Context, userID string) ([]Role, error) {
	if err := p.EnsureDefaultGuruRole(ctx, userID); err != nil {
		return nil, err
	}
	var out []Role
	err := withPersistRetry(ctx, func() error {
		rows, err := p.Pool.Query(ctx, `
			SELECT id, name, description
			FROM public.roles
			WHERE user_id = $1::uuid
			ORDER BY name ASC`,
			userID,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		out = nil
		for rows.Next() {
			var r Role
			var desc pgtype.Text
			if err := rows.Scan(&r.ID, &r.Name, &desc); err != nil {
				return err
			}
			if desc.Valid {
				s := desc.String
				r.Description = &s
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// InsertRole adds a role row for userID.
func (p *Persist) InsertRole(ctx context.Context, userID, name string, description *string) (Role, error) {
	var r Role
	err := withPersistRetry(ctx, func() error {
		var desc pgtype.Text
		err := p.Pool.QueryRow(ctx, `
			INSERT INTO public.roles (user_id, name, description, created_at, last_updated)
			VALUES ($1::uuid, $2, $3, now(), now())
			RETURNING id, name, description`,
			userID, name, description,
		).Scan(&r.ID, &r.Name, &desc)
		if err != nil {
			return err
		}
		if desc.Valid {
			s := desc.String
			r.Description = &s
		}
		return nil
	})
	return r, err
}

// UpdateRoleDescription updates description and last_updated if the row belongs to userID.
func (p *Persist) UpdateRoleDescription(ctx context.Context, userID string, roleID int64, description *string) error {
	return withPersistRetry(ctx, func() error {
		tag, err := p.Pool.Exec(ctx, `
			UPDATE public.roles
			SET description = $1, last_updated = now()
			WHERE id = $2 AND user_id = $3::uuid`,
			description, roleID, userID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("role not found or access denied")
		}
		return nil
	})
}

// DeleteRole removes a role if it belongs to userID.
func (p *Persist) DeleteRole(ctx context.Context, userID string, roleID int64) error {
	return withPersistRetry(ctx, func() error {
		tag, err := p.Pool.Exec(ctx, `
			DELETE FROM public.roles
			WHERE id = $1 AND user_id = $2::uuid`,
			roleID, userID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("role not found or access denied")
		}
		return nil
	})
}
