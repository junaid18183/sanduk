package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"filippo.io/age"
	"github.com/99designs/keyring"
)

// ageFileVersion is the on-disk format version for the age-encrypted store.
const ageFileVersion = 1

// ageKeyring is an encrypted-file backend that keeps every item in a single
// age-encrypted JSON document. It is intended for headless environments where
// no OS keystore is available. The passphrase is scrypt-derived by age, and the
// document is written with an atomic rename.
//
// It implements keyring.Keyring, so it composes with the rest of sanduk
// exactly like the native backends.
type ageKeyring struct {
	path     string
	passFunc func(string) (string, error)

	mu     sync.Mutex
	loaded bool
	items  map[string]keyring.Item
	pass   string // cached for the lifetime of the process
}

func newAgeKeyring(path string, passFunc func(string) (string, error)) *ageKeyring {
	return &ageKeyring{path: path, passFunc: passFunc}
}

type ageFile struct {
	Version int                `json:"version"`
	Items   map[string]ageItem `json:"items"`
}

type ageItem struct {
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Data        []byte `json:"data"`
}

// passphrase resolves the passphrase once and caches it, so a single command
// never prompts more than once. When the vault is being created the passphrase
// is asked for twice: a typo here would otherwise lock the vault under a
// passphrase the user does not know. The confirm is a no-op for the
// non-interactive (environment variable) source, which returns instantly.
func (k *ageKeyring) passphrase() (string, error) {
	if k.pass != "" {
		return k.pass, nil
	}
	// A missing or empty file is a new vault; confirm the passphrase for
	// both, or a typo locks the vault under a passphrase nobody knows.
	creating := false
	if fi, err := os.Stat(k.path); errors.Is(err, os.ErrNotExist) || (err == nil && fi.Size() == 0) {
		creating = true
	}

	p, err := k.passFunc(fmt.Sprintf("Enter passphrase for %s: ", filepath.Base(k.path)))
	if err != nil {
		return "", err
	}
	if p == "" {
		return "", errors.New("empty passphrase")
	}
	if creating {
		again, err := k.passFunc(fmt.Sprintf("Confirm passphrase for %s: ", filepath.Base(k.path)))
		if err != nil {
			return "", err
		}
		if again != p {
			return "", errors.New("passphrases do not match")
		}
	}
	k.pass = p
	return p, nil
}

// withLock runs fn while holding an OS-level exclusive lock on the vault file,
// so two concurrent sanduk processes cannot interleave a read-modify-write and
// lose one of the updates.
func (k *ageKeyring) withLock(fn func() error) error {
	lk, err := lockPath(k.path)
	if err != nil {
		return err
	}
	defer lk.release()
	return fn()
}

func (k *ageKeyring) load() error {
	if k.loaded {
		return nil
	}
	k.items = map[string]keyring.Item{}

	ciphertext, err := os.ReadFile(k.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			k.loaded = true
			return nil
		}
		return err
	}
	if len(ciphertext) == 0 {
		k.loaded = true
		return nil
	}
	if err := checkPrivateFile(k.path); err != nil {
		// The file is encrypted, so this is a warning rather than a refusal:
		// a readable copy is still exposed to offline passphrase guessing.
		fmt.Fprintf(os.Stderr, "sanduk: warning: %v\n", err)
	}

	pass, err := k.passphrase()
	if err != nil {
		return err
	}
	identity, err := age.NewScryptIdentity(pass)
	if err != nil {
		return err
	}
	r, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		return fmt.Errorf("could not decrypt %s (wrong passphrase?): %w", k.path, err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	var f ageFile
	if err := json.Unmarshal(plain, &f); err != nil {
		return fmt.Errorf("could not parse %s: %w", k.path, err)
	}
	for name, it := range f.Items {
		k.items[name] = keyring.Item{
			Key:         name,
			Label:       it.Label,
			Description: it.Description,
			Data:        it.Data,
		}
	}
	k.loaded = true
	return nil
}

func (k *ageKeyring) save() error {
	f := ageFile{Version: ageFileVersion, Items: make(map[string]ageItem, len(k.items))}
	for name, it := range k.items {
		f.Items[name] = ageItem{Label: it.Label, Description: it.Description, Data: it.Data}
	}
	plain, err := json.Marshal(f)
	if err != nil {
		return err
	}

	pass, err := k.passphrase()
	if err != nil {
		return err
	}
	recipient, err := age.NewScryptRecipient(pass)
	if err != nil {
		return err
	}
	if testScryptWorkFactor > 0 {
		recipient.SetWorkFactor(testScryptWorkFactor)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return err
	}
	if _, err := w.Write(plain); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	// The temp-write, fsync and rename live in WriteFileAtomic so a crash cannot
	// truncate the vault and a reader always sees a complete document.
	return WriteFileAtomic(k.path, buf.Bytes(), 0o600)
}

// Get implements keyring.Keyring.
func (k *ageKeyring) Get(key string) (keyring.Item, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.load(); err != nil {
		return keyring.Item{}, err
	}
	it, ok := k.items[key]
	if !ok {
		return keyring.Item{}, keyring.ErrKeyNotFound
	}
	return it, nil
}

// GetMetadata implements keyring.Keyring. It returns the non-secret parts of
// an item.
func (k *ageKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.load(); err != nil {
		return keyring.Metadata{}, err
	}
	it, ok := k.items[key]
	if !ok {
		return keyring.Metadata{}, keyring.ErrKeyNotFound
	}
	meta := keyring.Metadata{
		Item: &keyring.Item{
			Key:         it.Key,
			Label:       it.Label,
			Description: it.Description,
		},
	}
	if info, err := os.Stat(k.path); err == nil {
		meta.ModificationTime = info.ModTime()
	}
	return meta, nil
}

// Set implements keyring.Keyring.
func (k *ageKeyring) Set(item keyring.Item) error {
	if item.Key == "" {
		return errors.New("keyring: item has no Key")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.withLock(func() error {
		// Another process may have written since we last loaded; re-read under
		// the lock so we merge into the current file, not a stale copy.
		k.loaded = false
		if err := k.load(); err != nil {
			return err
		}
		k.items[item.Key] = keyring.Item{
			Key:         item.Key,
			Label:       item.Label,
			Description: item.Description,
			Data:        item.Data,
		}
		return k.save()
	})
}

// Remove implements keyring.Keyring.
func (k *ageKeyring) Remove(key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.withLock(func() error {
		k.loaded = false
		if err := k.load(); err != nil {
			return err
		}
		if _, ok := k.items[key]; !ok {
			return keyring.ErrKeyNotFound
		}
		delete(k.items, key)
		return k.save()
	})
}

// Keys implements keyring.Keyring.
func (k *ageKeyring) Keys() ([]string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.load(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(k.items))
	for name := range k.items {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys, nil
}
