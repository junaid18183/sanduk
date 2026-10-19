//go:build darwin && cgo

// Package vault: macOS keychain backend that requires user presence (Touch ID
// or the login password) for every read.
//
// EXPERIMENTAL. This backend is compiled only on macOS with cgo enabled. It has
// not been exercised against a live biometric prompt in this repository, so
// verify it on your own machine before relying on it.
//
// It stores items under a separate service name ("<service>-biometric") so it
// never mixes with items written by the standard keychain backend.
package vault

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation -framework LocalAuthentication
#include <stdlib.h>
#include <string.h>
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>

#include "biometric_auth_darwin.h"

// authReuseSeconds is how long one successful Touch ID may cover further reads.
// It is deliberately short: long enough for a single command to resolve a group
// and the secrets it links, far too short to leave the vault unlocked behind
// the user.
#define SANDUK_AUTH_REUSE_SECONDS 10.0

// sanduk_cfstring_to_cstr returns a malloc'd UTF-8 copy of s, or NULL.
static char *sanduk_cfstring_to_cstr(CFStringRef s) {
	if (s == NULL) {
		return NULL;
	}
	CFIndex len = CFStringGetLength(s);
	CFIndex max = CFStringGetMaximumSizeForEncoding(len, kCFStringEncodingUTF8) + 1;
	char *buf = (char *)malloc((size_t)max);
	if (buf == NULL) {
		return NULL;
	}
	if (!CFStringGetCString(s, buf, max, kCFStringEncodingUTF8)) {
		free(buf);
		return NULL;
	}
	return buf;
}

static char *sanduk_cferror_to_cstr(CFErrorRef cfErr) {
	if (cfErr == NULL) {
		return NULL;
	}
	CFStringRef desc = CFErrorCopyDescription(cfErr);
	char *msg = sanduk_cfstring_to_cstr(desc);
	if (desc != NULL) {
		CFRelease(desc);
	}
	return msg;
}

static char *sanduk_status_error(OSStatus status) {
	CFStringRef s = SecCopyErrorMessageString(status, NULL);
	char *msg = sanduk_cfstring_to_cstr(s);
	if (s != NULL) {
		CFRelease(s);
	}
	if (msg == NULL) {
		msg = (char *)malloc(48);
		if (msg != NULL) {
			snprintf(msg, 48, "OSStatus %d", (int)status);
		}
	}
	return msg;
}

// sanduk_new_query builds a generic-password query for service (+ optional account).
static CFMutableDictionaryRef sanduk_new_query(const char *service, const char *account) {
	CFStringRef svc = CFStringCreateWithCString(kCFAllocatorDefault, service, kCFStringEncodingUTF8);
	if (svc == NULL) {
		return NULL;
	}
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (q == NULL) {
		CFRelease(svc);
		return NULL;
	}
	CFDictionaryAddValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionaryAddValue(q, kSecAttrService, svc);
	CFRelease(svc);

	if (account != NULL) {
		CFStringRef acc = CFStringCreateWithCString(kCFAllocatorDefault, account, kCFStringEncodingUTF8);
		if (acc == NULL) {
			CFRelease(q);
			return NULL;
		}
		CFDictionaryAddValue(q, kSecAttrAccount, acc);
		CFRelease(acc);
	}
	return q;
}

static void sanduk_delete_existing(const char *service, const char *account) {
	CFMutableDictionaryRef q = sanduk_new_query(service, account);
	if (q == NULL) {
		return;
	}
	SecItemDelete(q);
	CFRelease(q);
}

// sanduk_set writes an item. Returns 0 on success, non-zero (OSStatus) on
// failure with *errmsg populated. require_presence attaches a user-presence
// access control so reads prompt for Touch ID / the login password.
static int sanduk_set(const char *service, const char *account,
                      const void *data, size_t len,
                      int require_presence, char **errmsg) {
	*errmsg = NULL;

	CFDataRef value = CFDataCreate(kCFAllocatorDefault, (const UInt8 *)data, (CFIndex)len);
	if (value == NULL) {
		*errmsg = strdup("out of memory");
		return -1;
	}

	CFMutableDictionaryRef q = sanduk_new_query(service, account);
	if (q == NULL) {
		CFRelease(value);
		*errmsg = strdup("could not build keychain query");
		return -1;
	}
	CFDictionaryAddValue(q, kSecValueData, value);

	CFStringRef label = CFStringCreateWithFormat(kCFAllocatorDefault, NULL, CFSTR("sanduk: %s"), account);
	if (label != NULL) {
		CFDictionaryAddValue(q, kSecAttrLabel, label);
		CFRelease(label);
	}

	if (require_presence) {
		CFErrorRef cfErr = NULL;
		SecAccessControlRef ac = SecAccessControlCreateWithFlags(
			kCFAllocatorDefault,
			kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
			kSecAccessControlUserPresence,
			&cfErr);
		if (ac == NULL) {
			char *msg = sanduk_cferror_to_cstr(cfErr);
			if (cfErr != NULL) {
				CFRelease(cfErr);
			}
			CFRelease(q);
			CFRelease(value);
			*errmsg = msg != NULL ? msg : strdup("could not create access control");
			return -1;
		}
		CFDictionaryAddValue(q, kSecAttrAccessControl, ac);
		CFRelease(ac);
	} else {
		CFDictionaryAddValue(q, kSecAttrAccessible, kSecAttrAccessibleWhenUnlockedThisDeviceOnly);
	}

	// Delete the old item only once everything needed for the replacement has
	// been built, so a failure above cannot lose the existing secret.
	sanduk_delete_existing(service, account);

	OSStatus st = SecItemAdd(q, NULL);
	CFRelease(q);
	CFRelease(value);

	if (st != errSecSuccess) {
		*errmsg = sanduk_status_error(st);
		return (int)st;
	}
	return 0;
}

// sanduk_get reads an item. Returns 0 on success, -2 if not found, otherwise
// non-zero (OSStatus) with *errmsg populated.
static int sanduk_get(const char *service, const char *account,
                      void **out, size_t *outlen, char **errmsg) {
	*errmsg = NULL;
	*out = NULL;
	*outlen = 0;

	CFMutableDictionaryRef q = sanduk_new_query(service, account);
	if (q == NULL) {
		*errmsg = strdup("could not build keychain query");
		return -1;
	}
	CFDictionaryAddValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionaryAddValue(q, kSecMatchLimit, kSecMatchLimitOne);

	// Reuse one authentication across the reads of a single command, so
	// resolving a group with links prompts once rather than once per secret.
	// Without a context each read authenticates separately, which still works.
	void *actx = sanduk_auth_context(SANDUK_AUTH_REUSE_SECONDS);
	if (actx != NULL) {
		CFDictionaryAddValue(q, kSecUseAuthenticationContext, (CFTypeRef)actx);
	}

	CFTypeRef result = NULL;
	OSStatus st = SecItemCopyMatching(q, &result);
	CFRelease(q);

	if (st == errSecItemNotFound) {
		return -2;
	}
	if (st != errSecSuccess) {
		*errmsg = sanduk_status_error(st);
		return (int)st;
	}

	CFDataRef d = (CFDataRef)result;
	CFIndex n = CFDataGetLength(d);
	void *buf = malloc((size_t)(n > 0 ? n : 1));
	if (buf == NULL) {
		CFRelease(result);
		*errmsg = strdup("out of memory");
		return -1;
	}
	CFDataGetBytes(d, CFRangeMake(0, n), (UInt8 *)buf);
	CFRelease(result);

	*out = buf;
	*outlen = (size_t)n;
	return 0;
}

// sanduk_meta checks existence using attributes only, avoiding the
// user-presence prompt. Returns 0 if present, -2 if absent.
static int sanduk_meta(const char *service, const char *account, char **errmsg) {
	*errmsg = NULL;

	CFMutableDictionaryRef q = sanduk_new_query(service, account);
	if (q == NULL) {
		*errmsg = strdup("could not build keychain query");
		return -1;
	}
	CFDictionaryAddValue(q, kSecReturnAttributes, kCFBooleanTrue);
	CFDictionaryAddValue(q, kSecMatchLimit, kSecMatchLimitOne);

	CFTypeRef result = NULL;
	OSStatus st = SecItemCopyMatching(q, &result);
	CFRelease(q);

	if (st == errSecItemNotFound) {
		return -2;
	}
	if (st != errSecSuccess) {
		*errmsg = sanduk_status_error(st);
		return (int)st;
	}
	CFRelease(result);
	return 0;
}

// sanduk_delete removes an item. Returns 0, -2 if not found, or an OSStatus.
static int sanduk_delete(const char *service, const char *account, char **errmsg) {
	*errmsg = NULL;

	CFMutableDictionaryRef q = sanduk_new_query(service, account);
	if (q == NULL) {
		*errmsg = strdup("could not build keychain query");
		return -1;
	}
	OSStatus st = SecItemDelete(q);
	CFRelease(q);

	if (st == errSecItemNotFound) {
		return -2;
	}
	if (st != errSecSuccess) {
		*errmsg = sanduk_status_error(st);
		return (int)st;
	}
	return 0;
}

// sanduk_keys lists the account names for a service using attributes only.
static int sanduk_keys(const char *service, char ***out, int *count, char **errmsg) {
	*errmsg = NULL;
	*out = NULL;
	*count = 0;

	CFMutableDictionaryRef q = sanduk_new_query(service, NULL);
	if (q == NULL) {
		*errmsg = strdup("could not build keychain query");
		return -1;
	}
	CFDictionaryAddValue(q, kSecReturnAttributes, kCFBooleanTrue);
	CFDictionaryAddValue(q, kSecMatchLimit, kSecMatchLimitAll);

	CFTypeRef result = NULL;
	OSStatus st = SecItemCopyMatching(q, &result);
	CFRelease(q);

	if (st == errSecItemNotFound) {
		return 0;
	}
	if (st != errSecSuccess) {
		*errmsg = sanduk_status_error(st);
		return (int)st;
	}

	CFArrayRef arr = (CFArrayRef)result;
	CFIndex n = CFArrayGetCount(arr);
	char **items = (char **)calloc((size_t)(n > 0 ? n : 1), sizeof(char *));
	if (items == NULL) {
		CFRelease(result);
		*errmsg = strdup("out of memory");
		return -1;
	}

	int k = 0;
	for (CFIndex i = 0; i < n; i++) {
		CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(arr, i);
		CFStringRef acc = (CFStringRef)CFDictionaryGetValue(dict, kSecAttrAccount);
		char *s = sanduk_cfstring_to_cstr(acc);
		if (s != NULL) {
			items[k++] = s;
		}
	}
	CFRelease(result);

	*out = items;
	*count = k;
	return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sort"
	"unsafe"

	"github.com/99designs/keyring"
)

// biometricSuffix namespaces this backend away from the standard keychain
// backend, which shares the same service name.
const biometricSuffix = "-biometric"

type biometricKeyring struct {
	service string
}

// newBiometricKeyring returns a keyring whose items require user presence on
// read. The service is suffixed so it does not collide with standard items.
func newBiometricKeyring(service string) (keyring.Keyring, error) {
	return &biometricKeyring{service: service + biometricSuffix}, nil
}

func takeCString(p *C.char) string {
	if p == nil {
		return "unknown error"
	}
	s := C.GoString(p)
	C.free(unsafe.Pointer(p))
	return s
}

func (k *biometricKeyring) cStrings(account string) (*C.char, *C.char) {
	svc := C.CString(k.service)
	var acc *C.char
	if account != "" {
		acc = C.CString(account)
	}
	return svc, acc
}

// Get implements keyring.Keyring. It prompts for user presence.
func (k *biometricKeyring) Get(key string) (keyring.Item, error) {
	if key == "" {
		// A NULL account matches every item in the service.
		return keyring.Item{}, errors.New("keyring: empty key")
	}
	svc, acc := k.cStrings(key)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))

	var out unsafe.Pointer
	var outlen C.size_t
	var cerr *C.char

	switch rc := C.sanduk_get(svc, acc, &out, &outlen, &cerr); rc {
	case 0:
		defer C.free(out)
		return keyring.Item{
			Key:   key,
			Label: "sanduk: " + key,
			Data:  C.GoBytes(out, C.int(outlen)),
		}, nil
	case -2:
		return keyring.Item{}, keyring.ErrKeyNotFound
	default:
		return keyring.Item{}, fmt.Errorf("keychain read failed: %s", takeCString(cerr))
	}
}

// GetMetadata implements keyring.Keyring using an attributes-only query, so it
// does not trigger a user-presence prompt.
func (k *biometricKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	if key == "" {
		// A NULL account matches every item in the service.
		return keyring.Metadata{}, errors.New("keyring: empty key")
	}
	svc, acc := k.cStrings(key)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))

	var cerr *C.char
	switch rc := C.sanduk_meta(svc, acc, &cerr); rc {
	case 0:
		return keyring.Metadata{Item: &keyring.Item{Key: key, Label: "sanduk: " + key}}, nil
	case -2:
		return keyring.Metadata{}, keyring.ErrKeyNotFound
	default:
		return keyring.Metadata{}, fmt.Errorf("keychain metadata failed: %s", takeCString(cerr))
	}
}

// Set implements keyring.Keyring, storing the item with a user-presence ACL.
func (k *biometricKeyring) Set(item keyring.Item) error {
	if item.Key == "" {
		return errors.New("keyring: item has no Key")
	}
	svc, acc := k.cStrings(item.Key)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))

	var data unsafe.Pointer
	if len(item.Data) > 0 {
		data = C.CBytes(item.Data)
		defer C.free(data)
	}

	var cerr *C.char
	if rc := C.sanduk_set(svc, acc, data, C.size_t(len(item.Data)), 1, &cerr); rc != 0 {
		return fmt.Errorf("keychain write failed: %s", takeCString(cerr))
	}
	return nil
}

// Remove implements keyring.Keyring.
func (k *biometricKeyring) Remove(key string) error {
	if key == "" {
		// A NULL account matches every item in the service.
		return errors.New("keyring: empty key")
	}
	svc, acc := k.cStrings(key)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))

	var cerr *C.char
	switch rc := C.sanduk_delete(svc, acc, &cerr); rc {
	case 0:
		return nil
	case -2:
		return keyring.ErrKeyNotFound
	default:
		return fmt.Errorf("keychain delete failed: %s", takeCString(cerr))
	}
}

// Keys implements keyring.Keyring using an attributes-only query.
func (k *biometricKeyring) Keys() ([]string, error) {
	svc, _ := k.cStrings("")
	defer C.free(unsafe.Pointer(svc))

	var out **C.char
	var count C.int
	var cerr *C.char

	if rc := C.sanduk_keys(svc, &out, &count, &cerr); rc != 0 {
		return nil, fmt.Errorf("keychain list failed: %s", takeCString(cerr))
	}
	defer C.free(unsafe.Pointer(out))

	n := int(count)
	keys := make([]string, 0, n)
	if n > 0 {
		for _, p := range unsafe.Slice(out, n) {
			keys = append(keys, takeCString(p))
		}
	}
	sort.Strings(keys)
	return keys, nil
}
