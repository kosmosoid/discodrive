// Package subsonic implements the OpenSubsonic REST API (https://opensubsonic.netlify.app/).
package subsonic

// Subsonic error codes as defined by the Subsonic / OpenSubsonic spec.
const (
	ErrGeneric           = 0
	ErrMissingParam      = 10
	ErrClientTooOld      = 20
	ErrServerTooOld      = 30
	ErrWrongAuth         = 40
	ErrTokenNotSupported = 41
	ErrNotAuthorized     = 50
	ErrNotFound          = 70
)
