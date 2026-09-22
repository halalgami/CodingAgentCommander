package imagegen

import (
	"fmt"

	"github.com/halalgami/CodingAgentCommander/internal/secrets"
)

// KeyRefPrefix namespaces this package's credentials inside the shared
// keychain service. Every other key this app stores uses a bare
// environment-variable name in one flat namespace, and a provider id here is a
// short word, so without a prefix a provider could overwrite a live credential.
const KeyRefPrefix = "imagegen."

// KeyRef is the keychain ref a provider's key is stored under.
func KeyRef(providerID string) string { return KeyRefPrefix + providerID }

// StoreKey saves a provider's API key in the OS keychain.
func StoreKey(providerID, key string) error {
	if providerID == "" {
		return fmt.Errorf("imagegen: no provider id")
	}
	if key == "" {
		// An empty value would read back as "a key is set" and fail later at
		// the provider with an unhelpful 401.
		return fmt.Errorf("imagegen: refusing to store an empty key")
	}
	return secrets.Set(KeyRef(providerID), key)
}

// LoadKey returns the stored key, or "" when none is stored. A missing key is
// not an error: prompting for one is the caller's job, and the keychain reports
// "not found" the same way it reports a locked keychain.
func LoadKey(providerID string) string {
	v, err := secrets.Get(KeyRef(providerID))
	if err != nil {
		return ""
	}
	return v
}

// ClearKey removes a provider's stored key.
func ClearKey(providerID string) error { return secrets.Delete(KeyRef(providerID)) }
