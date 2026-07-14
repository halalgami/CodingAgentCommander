// Package secrets stores provider API keys in the OS keychain.
package secrets

import "github.com/zalando/go-keyring"

const service = "commander"

// Set stores key under ref in the OS keychain.
func Set(ref, key string) error { return keyring.Set(service, ref, key) }

// Get retrieves the key stored under ref.
func Get(ref string) (string, error) { return keyring.Get(service, ref) }

// Delete removes the key stored under ref.
func Delete(ref string) error { return keyring.Delete(service, ref) }
