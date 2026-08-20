//go:build !windows

package deps

import (
	"context"
	"errors"
)

// PwshVersion is the pinned PowerShell 7 release. Declared here too so the
// cross-platform App bindings compile on macOS.
const PwshVersion = "7.6.5"

// ErrPwshNotNeeded is returned by InstallPwsh off Windows. pwsh is not a
// Commander dependency on macOS: it is required by psmux's Claude Code
// integration, and psmux is Windows-only.
var ErrPwshNotNeeded = errors.New("PowerShell 7 is only required on Windows")

// ManagedPwshDir and ManagedPwsh have no meaning off Windows; they return empty
// so callers that build a PATH entry from them contribute nothing.
func ManagedPwshDir() string { return "" }

// ManagedPwsh is the empty counterpart of the Windows implementation.
func ManagedPwsh() string { return "" }

// InstallPwsh always fails off Windows. The UI never offers it there — Status
// omits pwsh from the dependency list — so this only guards the binding.
func InstallPwsh(context.Context, func(string)) error { return ErrPwshNotNeeded }
