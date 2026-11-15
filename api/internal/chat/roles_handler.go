package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

type rolePostBody struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type rolePatchBody struct {
	Description *string `json:"description"`
}

func normalizeRoleDescription(input *string) (string, bool) {
	if input == nil {
		return "", false
	}
	desc := strings.TrimSpace(*input)
	if desc == "" {
		return "", false
	}
	maxChars := roleDescriptionMaxChars()
	if utf8.RuneCountInString(desc) > maxChars {
		return "", false
	}
	return desc, true
}

// rolesDBError logs err and maps common Postgres codes to clear responses; otherwise writes fallback.
func rolesDBError(w http.ResponseWriter, err error, operation, fallback string) {
	log.Printf("roles %s: %v", operation, err)
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "42P01":
			http.Error(
				w,
				`database has no public.roles table; apply migrations (see supabase/migrations)`,
				http.StatusInternalServerError,
			)
			return
		case "23505":
			http.Error(w, "a role with this name already exists", http.StatusConflict)
			return
		case "23514":
			http.Error(w, "role failed validation constraints", http.StatusBadRequest)
			return
		}
	}
	http.Error(w, fallback, http.StatusInternalServerError)
}

func rolesRequire(persist *Persist, auth *Authenticator, w http.ResponseWriter, r *http.Request) (userID string, ok bool) {
	uid, err := auth.UserID(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	if persist == nil || persist.Pool == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return "", false
	}
	return uid, true
}

// RolesCollectionHandler: GET (list), POST (create) /api/roles
func RolesCollectionHandler(persist *Persist, auth *Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := rolesRequire(persist, auth, w, r)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			limit, offset, ok := parseListPageParams(r)
			if !ok {
				http.Error(w, "invalid pagination params", http.StatusBadRequest)
				return
			}
			roles, err := persist.ListRoles(r.Context(), userID, limit, offset)
			if err != nil {
				rolesDBError(w, err, "list", "could not list roles")
				return
			}
			if roles == nil {
				roles = []Role{}
			}
			for i := range roles {
				roles[i] = clampRoleForDisplay(roles[i])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(roles)

		case http.MethodPost:
			var body rolePostBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			name := strings.TrimSpace(body.Name)
			if name == "" {
				http.Error(w, "name required", http.StatusBadRequest)
				return
			}
			maxChars := roleNameMaxChars()
			if utf8.RuneCountInString(name) > maxChars {
				http.Error(w, fmt.Sprintf("name too long (max %d characters)", maxChars), http.StatusBadRequest)
				return
			}
			desc, ok := normalizeRoleDescription(body.Description)
			if !ok {
				http.Error(
					w,
					fmt.Sprintf("description is required and must be %d characters or fewer", roleDescriptionMaxChars()),
					http.StatusBadRequest,
				)
				return
			}
			row, err := persist.InsertRole(r.Context(), userID, name, &desc)
			if err != nil {
				rolesDBError(w, err, "create", "could not create role")
				return
			}
			row = clampRoleForDisplay(row)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(row)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// RolesItemHandler: PATCH, DELETE /api/roles/{id}
func RolesItemHandler(persist *Persist, auth *Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := rolesRequire(persist, auth, w, r)
		if !ok {
			return
		}
		idStr := r.PathValue("id")
		roleID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || roleID < 1 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPatch:
			var body rolePatchBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			desc, ok := normalizeRoleDescription(body.Description)
			if !ok {
				http.Error(
					w,
					fmt.Sprintf("description is required and must be %d characters or fewer", roleDescriptionMaxChars()),
					http.StatusBadRequest,
				)
				return
			}
			err = persist.UpdateRoleDescription(r.Context(), userID, roleID, &desc)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				rolesDBError(w, err, "update", "could not update role")
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case http.MethodDelete:
			err = persist.DeleteRole(r.Context(), userID, roleID)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				rolesDBError(w, err, "delete", "could not delete role")
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
