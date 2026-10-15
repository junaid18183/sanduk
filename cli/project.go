package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
)

// loadProject finds the nearest .sanduk.toml at or above the working directory.
func loadProject() (string, *vault.Project, error) {
	return vault.LoadProject(".")
}

// defaultEntry resolves the repository's default vault entry: the entry and
// optional key subset named by .sanduk.toml. It fails with an actionable message
// when there is no project file, so commands that can omit the entry say why
// they cannot.
func defaultEntry() (entry string, only []string, err error) {
	_, proj, err := loadProject()
	if err != nil {
		return "", nil, err
	}
	if proj == nil {
		return "", nil, fmt.Errorf("no entry given and no %s found in this directory or its parents",
			vault.ProjectFileName)
	}
	return proj.Entry, proj.Only, nil
}
