package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
)

// defaultEntry resolves the repository's default vault entry: the entry and
// optional key subset named by .sanduk.toml. It fails with an actionable message
// when there is no project file, so commands that can omit the entry say why
// they cannot.
//
// The project file must be approved first. A cloned repository can otherwise
// commit an entry name of its choosing, and "sanduk exec -- <its scripts>" would
// hand that entry's secrets to the repository. Approval is a hash of the file's
// contents, so editing it requires re-approving it.
func (a *app) defaultEntry() (entry string, only []string, err error) {
	path, proj, err := vault.LoadProject(".")
	if err != nil {
		return "", nil, err
	}
	if proj == nil {
		return "", nil, fmt.Errorf("no entry given and no %s found in this directory or its parents",
			vault.ProjectFileName)
	}
	if err := a.requireTrustedProject(path, proj); err != nil {
		return "", nil, err
	}
	return proj.Entry, proj.Only, nil
}

// requireTrustedProject refuses an unapproved (or edited) project file and tells
// the user how to approve it.
func (a *app) requireTrustedProject(path string, proj *vault.Project) error {
	ok, err := vault.IsProjectTrusted(a.trustPath(), proj)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s is not approved; review it, then run \"sanduk allow\" to trust entry %q",
			path, proj.Entry)
	}
	return nil
}
