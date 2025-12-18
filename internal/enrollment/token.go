package enrollment

import (
	"context"
)

// Token represents an enrollment token used for registering new agents.
type Token struct {
	Value     string
	ExpiresAt int64
}

// ValidateToken validates an enrollment token.
func ValidateToken(ctx context.Context, token string) (*Token, error) {
	// TODO: Implementar validación de token con el backend
	return &Token{
		Value: token,
	}, nil
}
