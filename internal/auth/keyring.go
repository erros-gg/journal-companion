package auth

import (
	keyring "github.com/zalando/go-keyring"
)

// keychainService is the service name used for all OS keychain entries.
// Changing this after shipping would silently orphan stored tokens.
const keychainService = "Journal Companion"

const keyToken = "device_token"
const keyUsername = "username"

// SaveToken stores the device token in the OS keychain.
func SaveToken(token string) error {
	return keyring.Set(keychainService, keyToken, token)
}

// LoadToken retrieves the stored device token. Returns ("", nil) if none is stored.
func LoadToken() (string, error) {
	t, err := keyring.Get(keychainService, keyToken)
	if err != nil {
		return "", nil // not found is not an error
	}
	return t, nil
}

// SaveUsername stores the display name associated with the current token.
func SaveUsername(username string) error {
	return keyring.Set(keychainService, keyUsername, username)
}

// LoadUsername retrieves the stored display name. Returns ("", nil) if none is stored.
func LoadUsername() (string, error) {
	u, err := keyring.Get(keychainService, keyUsername)
	if err != nil {
		return "", nil
	}
	return u, nil
}

// DeleteCredentials removes both the token and username from the keychain.
func DeleteCredentials() error {
	_ = keyring.Delete(keychainService, keyUsername)
	return keyring.Delete(keychainService, keyToken)
}
