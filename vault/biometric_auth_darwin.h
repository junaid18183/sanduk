// Shared LocalAuthentication context for the keychain-biometric backend.
//
// Each SecItemCopyMatching against a user-presence item authenticates on its
// own, so a group that links two secrets would raise one Touch ID prompt per
// read. Passing the same LAContext to every read lets macOS reuse a single
// successful authentication for a short window, which collapses one command's
// burst of reads into one prompt.

#ifndef SANDUK_BIOMETRIC_AUTH_H
#define SANDUK_BIOMETRIC_AUTH_H

// sanduk_auth_context returns a process-wide LAContext whose authentication may
// be reused for reuse_seconds, or NULL if one could not be created (in which
// case the caller simply omits it and each read prompts separately).
void *sanduk_auth_context(double reuse_seconds);

#endif
