package util

import "testing"

func TestSecretCryptoRoundTripAndPlaintextCompatibility(t *testing.T) {
	encrypted, err := EncryptSecret("sk-sensitive", "stable-test-key")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "sk-sensitive" || len(encrypted) < len("enc:v1:") || encrypted[:7] != "enc:v1:" {
		t.Fatalf("secret was not encrypted: %q", encrypted)
	}
	plain, err := DecryptSecret(encrypted, "stable-test-key")
	if err != nil || plain != "sk-sensitive" {
		t.Fatalf("round trip = %q, %v", plain, err)
	}
	legacy, err := DecryptSecret("legacy-plaintext", "stable-test-key")
	if err != nil || legacy != "legacy-plaintext" {
		t.Fatalf("legacy plaintext = %q, %v", legacy, err)
	}
}

func TestSecretCryptoRejectsWrongKey(t *testing.T) {
	encrypted, err := EncryptSecret("sk-sensitive", "correct-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptSecret(encrypted, "wrong-key"); err == nil {
		t.Fatal("wrong key should fail authentication")
	}
}
