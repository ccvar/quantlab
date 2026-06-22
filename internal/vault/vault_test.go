package vault

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestEncryptDecryptCredential(t *testing.T) {
	input := CredentialInput{
		Exchange:   "Binance",
		Label:      "desk one",
		APIKey:     "AKIA1234567890XYZ",
		Secret:     "super-secret-value",
		Passphrase: "correct horse battery",
		Permissions: Permissions{
			Trade: true,
		},
	}

	encrypted, err := EncryptCredential(input, time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EncryptCredential() error = %v", err)
	}
	if encrypted.APIKeyMask != "AKIA...0XYZ" {
		t.Fatalf("APIKeyMask = %q", encrypted.APIKeyMask)
	}
	if bytes.Contains(encrypted.Ciphertext, []byte(input.APIKey)) {
		t.Fatal("ciphertext contains the api key")
	}
	if bytes.Contains(encrypted.Ciphertext, []byte(input.Secret)) {
		t.Fatal("ciphertext contains the api secret")
	}

	plain, err := DecryptCredential(encrypted, input.Passphrase)
	if err != nil {
		t.Fatalf("DecryptCredential() error = %v", err)
	}
	if plain.APIKey != input.APIKey || plain.Secret != input.Secret {
		t.Fatalf("plain = %#v", plain)
	}
}

func TestDecryptCredentialRejectsWrongPassphrase(t *testing.T) {
	encrypted, err := EncryptCredential(CredentialInput{
		Exchange:      "OKX",
		Label:         "demo",
		APIKey:        "okx-key-123456",
		Secret:        "okx-secret",
		APIPassphrase: "okx-api-pass",
		Passphrase:    "correct horse battery",
	}, time.Time{})
	if err != nil {
		t.Fatalf("EncryptCredential() error = %v", err)
	}

	if _, err := DecryptCredential(encrypted, "wrong horse battery"); err == nil {
		t.Fatal("DecryptCredential() with wrong passphrase succeeded")
	}
}

func TestEncryptCredentialRequiresOKXAPIPassphrase(t *testing.T) {
	_, err := EncryptCredential(CredentialInput{
		Exchange:   "OKX",
		APIKey:     "abc123456",
		Secret:     "secret",
		Passphrase: "correct horse battery",
	}, time.Time{})
	if err == nil || err.Error() != "okx api passphrase is required" {
		t.Fatalf("error = %v, want okx api passphrase requirement", err)
	}
}

func TestEncryptCredentialRejectsWithdrawalPermission(t *testing.T) {
	_, err := EncryptCredential(CredentialInput{
		Exchange:   "Binance",
		APIKey:     "abc123456",
		Secret:     "secret",
		Passphrase: "correct horse battery",
		Permissions: Permissions{
			Withdraw: true,
		},
	}, time.Time{})
	if !errors.Is(err, ErrWithdrawalPermission) {
		t.Fatalf("error = %v, want ErrWithdrawalPermission", err)
	}
}

func TestEncryptCredentialRejectsWeakPassphrase(t *testing.T) {
	_, err := EncryptCredential(CredentialInput{
		Exchange:   "Binance",
		APIKey:     "abc123456",
		Secret:     "secret",
		Passphrase: "short",
	}, time.Time{})
	if !errors.Is(err, ErrWeakPassphrase) {
		t.Fatalf("error = %v, want ErrWeakPassphrase", err)
	}
}
