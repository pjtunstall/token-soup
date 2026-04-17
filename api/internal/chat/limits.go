package chat

import (
	"os"
	"strconv"
	"strings"
)

const defaultRoleNameMaxChars = 64
const defaultRoleDescriptionMaxChars = 2000
const defaultUserMessageMaxChars = 12000

func roleNameMaxChars() int {
	raw := strings.TrimSpace(os.Getenv("PUBLIC_ROLE_NAME_MAX_CHARS"))
	return envPositiveIntOrDefault(raw, defaultRoleNameMaxChars)
}

func userMessageMaxChars() int {
	raw := strings.TrimSpace(os.Getenv("PUBLIC_USER_MESSAGE_MAX_CHARS"))
	return envPositiveIntOrDefault(raw, defaultUserMessageMaxChars)
}

func roleDescriptionMaxChars() int {
	raw := strings.TrimSpace(os.Getenv("PUBLIC_ROLE_DESCRIPTION_MAX_CHARS"))
	return envPositiveIntOrDefault(raw, defaultRoleDescriptionMaxChars)
}

func envPositiveIntOrDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return fallback
	}
	return v
}
