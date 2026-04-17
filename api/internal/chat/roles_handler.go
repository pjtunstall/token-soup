package chat

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

type rolePostBody struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type rolePatchBody struct {
	Description *string `json:"description"`
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
		}
	}
	http.Error(w, fallback, http.StatusInternalServerError)
}

func rolesRequire(persist *Persist, cache *jwk.Cache, jwksURL string, w http.ResponseWriter, r *http.Request) (userID string, ok bool) {
	uid, err := AuthUserID(r.Context(), cache, jwksURL, r.Header.Get("Authorization"))
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
func RolesCollectionHandler(persist *Persist, cache *jwk.Cache, jwksURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := rolesRequire(persist, cache, jwksURL, w, r)
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
			row, err := persist.InsertRole(r.Context(), userID, name, body.Description)
			if err != nil {
				rolesDBError(w, err, "create", "could not create role")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(row)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// RolesItemHandler: PATCH, DELETE /api/roles/{id}
func RolesItemHandler(persist *Persist, cache *jwk.Cache, jwksURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := rolesRequire(persist, cache, jwksURL, w, r)
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
			err = persist.UpdateRoleDescription(r.Context(), userID, roleID, body.Description)
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
