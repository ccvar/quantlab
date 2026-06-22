package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	KDFPBKDF2SHA256   = "pbkdf2-sha256"
	DefaultIterations = 210_000
	keyLength         = 32
	minPassphraseLen  = 12
)

var (
	ErrWithdrawalPermission = errors.New("withdrawal permission is not allowed")
	ErrWeakPassphrase       = errors.New("passphrase must be at least 12 characters")
)

type Permissions struct {
	Read     bool `json:"read"`
	Trade    bool `json:"trade"`
	Withdraw bool `json:"withdraw"`
}

type CredentialInput struct {
	Exchange      string      `json:"exchange"`
	Label         string      `json:"label"`
	APIKey        string      `json:"apiKey"`
	Secret        string      `json:"secret"`
	APIPassphrase string      `json:"apiPassphrase,omitempty"`
	Passphrase    string      `json:"passphrase"`
	Permissions   Permissions `json:"permissions"`
}

type PlainCredential struct {
	APIKey        string `json:"apiKey"`
	Secret        string `json:"secret"`
	APIPassphrase string `json:"apiPassphrase,omitempty"`
}

type EncryptedCredential struct {
	ID            int64       `json:"id"`
	Exchange      string      `json:"exchange"`
	Label         string      `json:"label"`
	APIKeyMask    string      `json:"apiKeyMask"`
	Permissions   Permissions `json:"permissions"`
	KDFName       string      `json:"kdfName"`
	KDFIterations int         `json:"kdfIterations"`
	Salt          []byte      `json:"salt"`
	Nonce         []byte      `json:"nonce"`
	Ciphertext    []byte      `json:"ciphertext"`
	CreatedAt     string      `json:"createdAt"`
	UpdatedAt     string      `json:"updatedAt"`
}

type CredentialMeta struct {
	ID          int64       `json:"id"`
	Exchange    string      `json:"exchange"`
	Label       string      `json:"label"`
	APIKeyMask  string      `json:"apiKeyMask"`
	Permissions Permissions `json:"permissions"`
	CreatedAt   string      `json:"createdAt"`
	UpdatedAt   string      `json:"updatedAt"`
}

func EncryptCredential(input CredentialInput, now time.Time) (EncryptedCredential, error) {
	normalized, err := ValidateCredentialInput(input)
	if err != nil {
		return EncryptedCredential{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}

	payload, err := json.Marshal(PlainCredential{
		APIKey:        normalized.APIKey,
		Secret:        normalized.Secret,
		APIPassphrase: normalized.APIPassphrase,
	})
	if err != nil {
		return EncryptedCredential{}, err
	}

	salt, err := randomBytes(16)
	if err != nil {
		return EncryptedCredential{}, err
	}
	key, err := deriveKey(normalized.Passphrase, salt, DefaultIterations)
	if err != nil {
		return EncryptedCredential{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedCredential{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedCredential{}, err
	}
	nonce, err := randomBytes(aead.NonceSize())
	if err != nil {
		return EncryptedCredential{}, err
	}

	timestamp := now.UTC().Format(time.RFC3339)
	return EncryptedCredential{
		Exchange:      normalized.Exchange,
		Label:         normalized.Label,
		APIKeyMask:    MaskAPIKey(normalized.APIKey),
		Permissions:   normalized.Permissions,
		KDFName:       KDFPBKDF2SHA256,
		KDFIterations: DefaultIterations,
		Salt:          salt,
		Nonce:         nonce,
		Ciphertext:    aead.Seal(nil, nonce, payload, associatedData(normalized.Exchange, normalized.Label)),
		CreatedAt:     timestamp,
		UpdatedAt:     timestamp,
	}, nil
}

func DecryptCredential(encrypted EncryptedCredential, passphrase string) (PlainCredential, error) {
	if encrypted.KDFName != KDFPBKDF2SHA256 {
		return PlainCredential{}, fmt.Errorf("unsupported kdf %q", encrypted.KDFName)
	}
	key, err := deriveKey(passphrase, encrypted.Salt, encrypted.KDFIterations)
	if err != nil {
		return PlainCredential{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return PlainCredential{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return PlainCredential{}, err
	}
	plaintext, err := aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, associatedData(encrypted.Exchange, encrypted.Label))
	if err != nil {
		return PlainCredential{}, err
	}
	var credential PlainCredential
	if err := json.Unmarshal(plaintext, &credential); err != nil {
		return PlainCredential{}, err
	}
	return credential, nil
}

func ValidateCredentialInput(input CredentialInput) (CredentialInput, error) {
	input.Exchange = strings.TrimSpace(input.Exchange)
	input.Label = strings.TrimSpace(input.Label)
	input.APIKey = strings.TrimSpace(input.APIKey)
	input.Secret = strings.TrimSpace(input.Secret)
	input.APIPassphrase = strings.TrimSpace(input.APIPassphrase)
	input.Passphrase = strings.TrimSpace(input.Passphrase)
	input.Permissions.Read = true

	if input.Exchange == "" {
		return CredentialInput{}, errors.New("exchange is required")
	}
	if input.Label == "" {
		input.Label = input.Exchange + " main"
	}
	if input.APIKey == "" {
		return CredentialInput{}, errors.New("api key is required")
	}
	if input.Secret == "" {
		return CredentialInput{}, errors.New("api secret is required")
	}
	if strings.EqualFold(input.Exchange, "OKX") && input.APIPassphrase == "" {
		return CredentialInput{}, errors.New("okx api passphrase is required")
	}
	if len(input.Passphrase) < minPassphraseLen {
		return CredentialInput{}, ErrWeakPassphrase
	}
	if input.Permissions.Withdraw {
		return CredentialInput{}, ErrWithdrawalPermission
	}
	return input, nil
}

func MaskAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return "****"
	}
	if len(value) <= 8 {
		return value[:2] + "..." + value[len(value)-2:]
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func (credential EncryptedCredential) Meta() CredentialMeta {
	return CredentialMeta{
		ID:          credential.ID,
		Exchange:    credential.Exchange,
		Label:       credential.Label,
		APIKeyMask:  credential.APIKeyMask,
		Permissions: credential.Permissions,
		CreatedAt:   credential.CreatedAt,
		UpdatedAt:   credential.UpdatedAt,
	}
}

func deriveKey(passphrase string, salt []byte, iterations int) ([]byte, error) {
	if iterations <= 0 {
		return nil, errors.New("kdf iterations must be positive")
	}
	return pbkdf2.Key(sha256.New, passphrase, salt, iterations, keyLength)
}

func associatedData(exchange string, label string) []byte {
	return []byte(strings.TrimSpace(exchange) + ":" + strings.TrimSpace(label))
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return nil, err
	}
	return value, nil
}
