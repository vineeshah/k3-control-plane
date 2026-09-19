package pki

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

const tokenPrefix = "K8"

// Token is the cluster join secret in k3s's "secure token" shape:
//
//	K8<sha256 of the CA bundle>::<secret>
//
// The CA hash lets a brand-new agent trust the controller on first contact
// without any certificate copied onto it: it downloads the CA, checks the hash
// against the token, and only then talks TLS. The secret proves to the
// controller that the agent is allowed to join.
type Token struct {
	CAHash string
	Secret string
}

func (t Token) String() string {
	return tokenPrefix + t.CAHash + "::" + t.Secret
}

func ParseToken(raw string) (Token, error) {
	raw = strings.TrimSpace(raw)
	rest, ok := strings.CutPrefix(raw, tokenPrefix)
	if !ok {
		return Token{}, errors.New("token must start with " + tokenPrefix)
	}
	hash, secret, ok := strings.Cut(rest, "::")
	if !ok || len(hash) != 64 || secret == "" {
		return Token{}, errors.New("token must look like K8<64 hex chars>::<secret>")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return Token{}, fmt.Errorf("token CA hash is not hex: %w", err)
	}
	return Token{CAHash: hash, Secret: secret}, nil
}

// LoadOrCreateToken reads the token at path or creates one for ca. If secret
// is non-empty (an operator-chosen token, like K3S_TOKEN) it is used instead
// of a random one. A stored token whose CA hash no longer matches is an error:
// every agent holding it would refuse this controller.
func LoadOrCreateToken(path string, ca *CA, secret string) (Token, error) {
	want := Fingerprint(ca.PEM)

	if data, err := os.ReadFile(path); err == nil {
		token, err := ParseToken(string(data))
		if err != nil {
			return Token{}, fmt.Errorf("parse %s: %w", path, err)
		}
		if token.CAHash != want {
			return Token{}, fmt.Errorf("token in %s pins a different CA", path)
		}
		if secret != "" && secret != token.Secret {
			return Token{}, fmt.Errorf("-token differs from the one stored in %s", path)
		}
		return token, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Token{}, err
	}

	if secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return Token{}, err
		}
		secret = hex.EncodeToString(buf)
	}
	token := Token{CAHash: want, Secret: secret}
	if err := WriteFile(path, []byte(token.String()+"\n"), 0o600); err != nil {
		return Token{}, err
	}
	return token, nil
}
