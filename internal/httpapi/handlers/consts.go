package handlers

// Message literals shared across more than one handler file. Centralised here
// so a given string lives in exactly one place (avoids duplicated-literal smells
// and keeps wording consistent across endpoints).
const (
	// Validation messages returned to clients / used as logValidation reasons.
	msgInvalidRequestBody = "invalid request body"
	msgInvalidBody        = "invalid body"
	msgInvalidColor       = "invalid color"

	// Not-found / auth messages reused by several handlers.
	msgMissingAuthClaims = "missing auth claims"
	msgTaskNotFound      = "task not found"
	msgContextNotFound   = "context not found"
)
