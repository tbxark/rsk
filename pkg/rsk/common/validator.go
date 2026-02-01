package common

import "fmt"

// MinTokenLength is the minimum required length for authentication tokens in bytes.
const MinTokenLength = 16

// ValidateToken checks if the provided token meets the minimum security requirements.
// Returns an error if the token is too short.
func ValidateToken(token []byte) error {
	if len(token) < MinTokenLength {
		return fmt.Errorf("token too short: provided %d bytes, minimum required is %d bytes", len(token), MinTokenLength)
	}
	return nil
}
