package vault

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// TrustFileName is the per-user record of approved project files.
const TrustFileName = "trusted.toml"

// TrustPath resolves the project trust store. It lives next to the metadata
// config so it inherits the same directory permissions and travels with a
// custom SANDUK_CONFIG (useful for tests).
func TrustPath() string {
	if p := os.Getenv("SANDUK_TRUST"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(ConfigPath()), TrustFileName)
}

// TrustStore records a content hash per approved .sanduk.toml path. A project
// file is trusted only while its bytes match the approved hash, so editing it
// revokes the approval and it must be reviewed (and re-approved) again.
type TrustStore struct {
	Files map[string]string `toml:"files,omitempty"`
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
	if err := toml.Unmarshal(b, t); err != nil {
		return nil, err
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
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// IsProjectTrusted reports whether path has been approved with its current
// contents.
func IsProjectTrusted(path string) (bool, error) {
	hash, err := HashFile(path)
	if err != nil {
		return false, err
	}
	t, err := loadTrust(TrustPath())
	if err != nil {
		return false, err
	}
	return t.Files[path] == hash, nil
}

// ApproveProject records the current contents of path as approved and returns
// the hash that was stored.
func ApproveProject(path string) (string, error) {
	hash, err := HashFile(path)
	if err != nil {
		return "", err
	}
	tp := TrustPath()
	t, err := loadTrust(tp)
	if err != nil {
		return "", err
	}
	if t.Files == nil {
		t.Files = map[string]string{}
	}
	t.Files[path] = hash
	if err := saveTrust(tp, t); err != nil {
		return "", err
	}
	return hash, nil
}

// TrustedPaths returns the approved project-file paths, sorted.
func TrustedPaths() ([]string, error) {
	t, err := loadTrust(TrustPath())
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
