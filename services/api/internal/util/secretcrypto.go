package util

import "strings"

const encryptedSecretPrefix = "enc:v1:"

// EncryptSecret keeps an explicit version prefix so existing plaintext values
// remain readable during rolling upgrades and future ciphers can coexist.
func EncryptSecret(value, secret string) (string, error) {
	if value == "" || strings.HasPrefix(value, encryptedSecretPrefix) {
		return value, nil
	}
	encrypted, err := EncryptCardCode(value, secret)
	if err != nil {
		return "", err
	}
	return encryptedSecretPrefix + encrypted, nil
}

func DecryptSecret(value, secret string) (string, error) {
	if value == "" || !strings.HasPrefix(value, encryptedSecretPrefix) {
		return value, nil
	}
	return DecryptCardCode(strings.TrimPrefix(value, encryptedSecretPrefix), secret)
}
