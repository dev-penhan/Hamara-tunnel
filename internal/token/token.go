package token

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const prefix = "HAMARA1"

func Encode(kind string, value any) (string, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind != "OFFER" && kind != "RESPONSE" {
		return "", errors.New("token kind must be OFFER or RESPONSE")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(data)
	sum := sha256.Sum256([]byte(kind + "." + body))
	return fmt.Sprintf("%s.%s.%s.%s", prefix, kind, body, hex.EncodeToString(sum[:8])), nil
}

func Decode(input, expectedKind string, dst any) error {
	input = strings.TrimSpace(input)
	parts := strings.Split(input, ".")
	if len(parts) != 4 || parts[0] != prefix {
		return errors.New("invalid Hamara token format")
	}
	kind := strings.ToUpper(expectedKind)
	if parts[1] != kind {
		return fmt.Errorf("expected a %s token, got %s", kind, parts[1])
	}
	sum := sha256.Sum256([]byte(parts[1] + "." + parts[2]))
	want := hex.EncodeToString(sum[:8])
	if !strings.EqualFold(parts[3], want) {
		return errors.New("token checksum failed; the token may be truncated or modified")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("invalid token encoding: %w", err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("invalid token payload: %w", err)
	}
	return nil
}
