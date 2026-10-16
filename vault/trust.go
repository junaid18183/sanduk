package vault

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// TrustFileName is the per-user record of approved project files.
const TrustFileName = "trusted.toml"

// TrustPath resolves the project trust store from the environment. See
// TrustPathFor.
func TrustPath() string { return TrustPathFor(ConfigPath()) }

// TrustPathFor resolves the trust store for a given metadata config path:
// SANDUK_TRUST when set, otherwise trusted.toml next to the config, so it
// inherits the same directory permissions and moves with --config.
func TrustPathFor(configPath string) string {
	if p := os.Getenv("SANDUK_TRUST"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(configPath), TrustFileName)
}

// TrustStore records a content hash per approved .sanduk.toml path. A project
// file is trusted only while its bytes match the approved hash, so editing it
// revokes the approval and it must be reviewed (and re-approved) again.
type TrustStore struct {
	Files map[string]string `toml:"files,omitempty"`
}

// checkTrustLocation refuses a trust store inside the project being judged. The
// store's location can be moved with SANDUK_TRUST or SANDUK_CONFIG, so a cloned
// repository's .envrc could otherwise point sanduk at a trusted.toml it ships
// and approve its own .sanduk.toml.
func checkTrustLocation(trustPath string, p *Project) error {
	abs, err := filepath.Abs(trustPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(filepath.Dir(p.path), abs)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing trust store %s: it is inside the project it would approve (check SANDUK_TRUST / SANDUK_CONFIG)", trustPath)
	}
	return nil
}

func loadTrust(path string) (*TrustStore, error) {
	t := &TrustStore{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return t, nil
		}
		return nil, err
	}
	if err := checkPrivateFile(path); err != nil {
		return nil, fmt.Errorf("trust store: %w", err)
	}
	if err := toml.Unmarshal(b, t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

func saveTrust(path string, t *TrustStore) error {
	var buf bytes.Buffer
	buf.WriteString("# sanduk approved project files — non-secret. Each value is the sha256\n")
	buf.WriteString("# of a .sanduk.toml you have reviewed. Editing that file revokes the approval.\n\n")
	if err := toml.NewEncoder(&buf).Encode(t); err != nil {
		return err
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o600)
}

// HashFile returns the hex-encoded sha256 of a file's contents.
func HashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(b), nil
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// IsProjectTrusted reports whether p, as loaded, is approved in the trust store
// at tp. It compares
// the hash of the bytes p was parsed from, never a fresh read of the file.
func IsProjectTrusted(tp string, p *Project) (bool, error) {
	if err := checkTrustLocation(tp, p); err != nil {
		return false, err
	}
	t, err := loadTrust(tp)
	if err != nil {
		return false, err
	}
	return p.hash != "" && t.Files[p.path] == p.hash, nil
}

// ApproveProject records p, as loaded, as approved in the trust store at tp and
// returns the hash that
// was stored — the hash of the bytes that were shown to the user, not of
// whatever the file holds by the time the store is written.
func ApproveProject(tp string, p *Project) (string, error) {
	if p.hash == "" || p.path == "" {
		return "", fmt.Errorf("project was not loaded from a file")
	}
	if err := checkTrustLocation(tp, p); err != nil {
		return "", err
	}
	lk, err := lockPath(tp)
	if err != nil {
		return "", err
	}
	defer lk.release()
	t, err := loadTrust(tp)
	if err != nil {
		return "", err
	}
	if t.Files == nil {
		t.Files = map[string]string{}
	}
	t.Files[p.path] = p.hash
	if err := saveTrust(tp, t); err != nil {
		return "", err
	}
	return p.hash, nil
}

// TrustedPaths returns the approved project-file paths, sorted.
func TrustedPaths(tp string) ([]string, error) {
	t, err := loadTrust(tp)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(t.Files))
	for p := range t.Files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}
