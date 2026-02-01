package common

import "errors"

// Standard errors for use with errors.Is.
var (
	ErrAuthFailed      = errors.New("authentication failed")
	ErrPortInUse       = errors.New("port already in use")
	ErrPortNotReserved = errors.New("port not reserved")
	ErrRateLimited     = errors.New("rate limited")
	ErrConnectionLimit = errors.New("connection limit reached")
	ErrInvalidToken    = errors.New("invalid token")
)
