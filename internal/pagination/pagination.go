package pagination

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Cursor encodes the position of the last item seen on a page.
// It is opaque to the client (base64-encoded JSON).
type Cursor struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

// Params carries validated pagination inputs into the repository layer.
type Params struct {
	Limit  int
	Cursor *Cursor
}

// Encode serialises a Cursor to a URL-safe opaque string.
func Encode(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Decode parses an opaque cursor string. Returns an error on malformed input.
func Decode(s string) (*Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ParseLimit converts a raw query-string value to a clamped limit.
func ParseLimit(s string) int {
	if s == "" {
		return DefaultLimit
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}
