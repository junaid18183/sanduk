package vault

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the non-secret metadata store. It is deliberately safe to read,
// commit, and version: it never contains secret values, only the mapping from
// vault entries to environment variable names plus human descriptions.
type Config struct {
	// Backend settings. These are defaults only: a --flag or an environment
	// variable always wins, so one config file can serve several machines with
	// different keystores. All three are optional.
	Backend  string `toml:"backend,omitempty"`
	Keychain string `toml:"keychain,omitempty"`
	Service  string `toml:"service,omitempty"`

	Secrets map[string]SecretMeta `toml:"secrets,omitempty"`
	Groups  map[string]GroupMeta  `toml:"groups,omitempty"`
	// Files describes file secrets. They are kept in their own table rather
	// than mixed in with Secrets so list can name the kind without reading
	// anything from the backend.
	Files map[string]SecretMeta `toml:"files,omitempty"`
}

// SecretMeta describes a flat secret entry.
type SecretMeta struct {
	Env         string `toml:"env,omitempty"`
	Description string `toml:"description,omitempty"`
	// Expires is a YYYY-MM-DD date after which the credential should be
	// rotated. It is a reminder, not an enforcement: sanduk never refuses to
	// inject an expired secret, because failing a deploy over a date in a
	// metadata file would be worse than the stale key it is warning about.
	Expires string `toml:"expires,omitempty"`
	// LinkedBy names the groups that link to this secret. It exists so a
	// listing can keep a group's building blocks out of the top level
	// without reading every group entry, which would decrypt the whole
	// vault on each list.
	//
	// It is a display hint and nothing resolves through it: the links
	// themselves live in the backend entry (Entry.Links), so a stale or
	// missing value here can only add or drop a row in a listing, never
	// change which variables an entry injects. "sanduk doctor --links --fix"
	// rebuilds it from the backend.
	LinkedBy []string `toml:"linked_by,omitempty"`
}

// GroupMeta describes a grouped entry. Group field names are the environment
// variable names, so no explicit mapping is required.
type GroupMeta struct {
	Description string `toml:"description,omitempty"`
	// Location is the directory this group belongs to: the repository it was
	// created in, or the one whose project file was approved. It is what
	// lets a listing show the group you are standing in and leave every
	// other repository's out.
	//
	// It is recorded here, in the user's own private config, rather than
	// taken from a repository, so no cloned repository can claim a group by
	// writing a file. A path that no longer exists simply stops matching.
	Location string `toml:"location,omitempty"`
	Expires  string `toml:"expires,omitempty"`
}

// LoadConfig reads the config file, returning an empty config if it does not
// exist yet.
func LoadConfig(path string) (*Config, error) {
	c := &Config{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// UpdateConfig loads the config at path, applies fn and saves the result, all
// under a cross-process lock. Without the lock two concurrent commands (two
// `sanduk add` in parallel scripts, say) each load, change and save the file,
// and one of the updates is lost.
func UpdateConfig(path string, fn func(*Config) error) error {
	lk, err := lockPath(path)
	if err != nil {
		return err
	}
	defer lk.release()
	c, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if err := fn(c); err != nil {
		return err
	}
	return SaveConfig(path, c)
}

// RecordSecret marks name as a flat secret, clearing any group record. An
// empty env or desc leaves the existing value alone.
func (c *Config) RecordSecret(name, env, desc string) {
	if c.Secrets == nil {
		c.Secrets = map[string]SecretMeta{}
	}
	m := c.Secrets[name]
	if env != "" {
		m.Env = env
	}
	if desc != "" {
		m.Description = desc
	}
	c.Secrets[name] = m
	delete(c.Groups, name)
	delete(c.Files, name)
}

// RecordFile marks name as a file secret, clearing any other record.
func (c *Config) RecordFile(name, env, desc string) {
	if c.Files == nil {
		c.Files = map[string]SecretMeta{}
	}
	m := c.Files[name]
	if env != "" {
		m.Env = env
	}
	if desc != "" {
		m.Description = desc
	}
	c.Files[name] = m
	delete(c.Secrets, name)
	delete(c.Groups, name)
}

// RecordGroup marks name as a group, clearing any secret record. An empty desc
// leaves the existing description alone.
func (c *Config) RecordGroup(name, desc string) {
	if c.Groups == nil {
		c.Groups = map[string]GroupMeta{}
	}
	g := c.Groups[name]
	if desc != "" {
		g.Description = desc
	}
	c.Groups[name] = g
	delete(c.Secrets, name)
	delete(c.Files, name)
}

// SetGroupLocation records the directory a group belongs to, or clears it when
// dir is empty. It reports whether a group record existed to record it on: a
// location describes a group sanduk already knows about.
func (c *Config) SetGroupLocation(name, dir string) bool {
	g, ok := c.Groups[name]
	if !ok {
		return false
	}
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return false
		}
		dir = filepath.Clean(abs)
	}
	g.Location = dir
	c.Groups[name] = g
	return true
}

// GroupLocations maps every group that belongs to an existing directory to
// that directory. A group with no location belongs nowhere in particular and
// is global.
//
// A location that is not there any more does not count. The directory may
// have been moved, deleted, or never cloned on this machine — and a group
// claimed by a directory that does not exist would otherwise be hidden from
// every listing, since no working directory can ever be inside it. Falling
// back to global means the worst case is a group you can still see.
func (c *Config) GroupLocations() map[string]string {
	if c == nil {
		return nil
	}
	out := make(map[string]string, len(c.Groups))
	for name, g := range c.Groups {
		if g.Location != "" && isDir(g.Location) {
			out[name] = g.Location
		}
	}
	return out
}

// isDir reports whether path is an existing directory.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// StaleLocations lists the groups whose recorded directory is gone, sorted.
// They still work — they are simply global again — so this is something
// doctor reports, never something a command fails on.
func (c *Config) StaleLocations() []string {
	var out []string
	for name, g := range c.Groups {
		if g.Location != "" && !isDir(g.Location) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// GroupAt returns the group whose location contains dir.
//
// A group matches dir and everything below it, so any subdirectory of a
// repository resolves to the same group — the same reach a project file has,
// which LoadProject finds by walking up. The deepest location wins, so a
// repository checked out inside another one keeps its own group.
func (c *Config) GroupAt(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	abs = filepath.Clean(abs)

	best, bestLen := "", -1
	for name, loc := range c.GroupLocations() {
		if !withinDir(loc, abs) || len(loc) < bestLen {
			continue
		}
		// Two groups on the same directory would otherwise depend on map
		// order; the names are sorted so the answer is at least stable.
		if len(loc) == bestLen && name > best {
			continue
		}
		best, bestLen = name, len(loc)
	}
	return best, best != ""
}

// withinDir reports whether path is root or lies under it. The separator check
// keeps /srv/app-old from matching a group located at /srv/app.
func withinDir(root, path string) bool {
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// RecordLink notes that group links to the flat secret target, so a listing
// can show target as part of that group rather than as a standalone entry. A
// target with no metadata record gets one: only a flat secret can be a link
// target, so its type is already known.
func (c *Config) RecordLink(target, group string) {
	if c.Secrets == nil {
		c.Secrets = map[string]SecretMeta{}
	}
	m := c.Secrets[target]
	for _, g := range m.LinkedBy {
		if g == group {
			return
		}
	}
	m.LinkedBy = append(m.LinkedBy, group)
	sort.Strings(m.LinkedBy)
	c.Secrets[target] = m
}

// RemoveLink forgets that group linked to target.
func (c *Config) RemoveLink(target, group string) {
	m, ok := c.Secrets[target]
	if !ok {
		return
	}
	m.LinkedBy = withoutString(m.LinkedBy, group)
	c.Secrets[target] = m
}

// SetLinkedBy replaces every linked_by hint with byTarget, which is the link
// graph as the backend actually holds it. It is how doctor repairs a mirror
// that drifted — a group edited on another machine, or a config restored from
// a backup.
func (c *Config) SetLinkedBy(byTarget map[string][]string) {
	for name, m := range c.Secrets {
		if len(m.LinkedBy) > 0 {
			m.LinkedBy = nil
			c.Secrets[name] = m
		}
	}
	for target, groups := range byTarget {
		for _, g := range groups {
			c.RecordLink(target, g)
		}
	}
}

// withoutString returns list without every occurrence of s, or nil when that
// empties it, so an empty list is omitted from the file rather than written
// as [].
func withoutString(list []string, s string) []string {
	var out []string
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

// ExpiryLayout is the date format used by the expires field. A plain date,
// not a timestamp: rotation reminders are a day-granularity concern, and a
// date is what a provider's dashboard shows.
const ExpiryLayout = "2006-01-02"

// ParseExpiry validates a YYYY-MM-DD expiry date.
func ParseExpiry(s string) (time.Time, error) {
	t, err := time.Parse(ExpiryLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("expiry %q is not a YYYY-MM-DD date", s)
	}
	return t, nil
}

// SetExpiry records (or with an empty date clears) an entry's expiry, in
// whichever metadata map already holds it. It reports whether the entry had a
// record to update: an entry with no metadata cannot carry an expiry, and
// silently creating one would guess at its type.
func (c *Config) SetExpiry(name, date string) bool {
	if m, ok := c.Secrets[name]; ok {
		m.Expires = date
		c.Secrets[name] = m
		return true
	}
	if m, ok := c.Files[name]; ok {
		m.Expires = date
		c.Files[name] = m
		return true
	}
	if g, ok := c.Groups[name]; ok {
		g.Expires = date
		c.Groups[name] = g
		return true
	}
	return false
}

// ExpiryOf returns the recorded expiry date for an entry, or "".
func (c *Config) ExpiryOf(name string) string {
	if c == nil {
		return ""
	}
	if m, ok := c.Secrets[name]; ok {
		return m.Expires
	}
	if m, ok := c.Files[name]; ok {
		return m.Expires
	}
	if g, ok := c.Groups[name]; ok {
		return g.Expires
	}
	return ""
}

// Expiry is one entry's rotation deadline, as doctor reports it.
type Expiry struct {
	Name string
	Date string
	Days int // whole days from now until Date; negative once past
}

// Expired reports whether the deadline has passed.
func (e Expiry) Expired() bool { return e.Days < 0 }

// Expiries returns every entry carrying an expiry date, sorted by how soon it
// falls due. Entries whose date does not parse are returned last with a zero
// day count, so a hand-edited config surfaces rather than being skipped.
// It reads only metadata: no value is ever decrypted to answer this.
func (c *Config) Expiries(now time.Time) ([]Expiry, error) {
	dates := map[string]string{}
	for name, m := range c.Secrets {
		if m.Expires != "" {
			dates[name] = m.Expires
		}
	}
	for name, m := range c.Files {
		if m.Expires != "" {
			dates[name] = m.Expires
		}
	}
	for name, g := range c.Groups {
		if g.Expires != "" {
			dates[name] = g.Expires
		}
	}

	today := now.UTC().Truncate(24 * time.Hour)
	out := make([]Expiry, 0, len(dates))
	for _, name := range sortedKeys(dates) {
		t, err := ParseExpiry(dates[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, Expiry{
			Name: name,
			Date: dates[name],
			Days: int(t.Sub(today).Hours() / 24),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Days < out[j].Days })
	return out, nil
}

// sortedKeys returns a map's keys in order, so output is deterministic.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Forget removes every metadata record for name, including any linked_by hint
// naming it: a removed group no longer owns the secrets it used to link.
func (c *Config) Forget(name string) {
	delete(c.Secrets, name)
	delete(c.Groups, name)
	delete(c.Files, name)
	for target, m := range c.Secrets {
		if len(m.LinkedBy) == 0 {
			continue
		}
		m.LinkedBy = withoutString(m.LinkedBy, name)
		c.Secrets[target] = m
	}
}

// SaveConfig writes the config file with restrictive permissions. The write is
// atomic, so a crash mid-save cannot truncate the metadata of every entry, and
// two concurrent writers cannot leave a half-written file behind.
func SaveConfig(path string, c *Config) error {
	var buf bytes.Buffer
	buf.WriteString("# sanduk config — metadata only. This file never contains secret values.\n\n")
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o600)
}

// EnvNameForSecret returns the environment variable a flat secret is exposed
// as: the configured name, or the upper-cased entry name.
func (c *Config) EnvNameForSecret(name string) string {
	if c != nil {
		if m, ok := c.Secrets[name]; ok && m.Env != "" {
			return m.Env
		}
		if m, ok := c.Files[name]; ok && m.Env != "" {
			return m.Env
		}
	}
	return strings.ToUpper(name)
}

// MetadataDrift compares the entry names a backend holds with the config's
// metadata. untracked entries exist in the backend with no metadata (list then
// guesses their type); orphaned records describe entries the backend no longer
// has. Both results are sorted.
func MetadataDrift(keys []string, c *Config) (untracked, orphaned []string) {
	inBackend := make(map[string]bool, len(keys))
	for _, k := range keys {
		inBackend[k] = true
		_, s := c.Secrets[k]
		_, g := c.Groups[k]
		_, f := c.Files[k]
		if !s && !g && !f {
			untracked = append(untracked, k)
		}
	}
	for name := range c.Secrets {
		if !inBackend[name] {
			orphaned = append(orphaned, name)
		}
	}
	for name := range c.Groups {
		if !inBackend[name] {
			orphaned = append(orphaned, name)
		}
	}
	for name := range c.Files {
		if !inBackend[name] {
			orphaned = append(orphaned, name)
		}
	}
	sort.Strings(untracked)
	sort.Strings(orphaned)
	return untracked, orphaned
}
