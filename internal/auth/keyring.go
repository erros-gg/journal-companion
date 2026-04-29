package auth

// Keyring service name used for all OS keychain operations.
// Changing this value after shipping would silently orphan stored tokens.
const keychainService = "Journal Companion"

// TODO Phase 2: implement Token, SaveToken, LoadToken, DeleteToken
// using github.com/zalando/go-keyring.
//
// func SaveToken(token string) error
// func LoadToken() (string, error)
// func DeleteToken() error
