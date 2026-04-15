package theme

import "fmt"

// Catalog provides theme discovery and loading from the themes directory.
// It wraps the existing ListOrdered and Load functions into a stateful type
// that can be injected as a dependency. Catalog does NOT own persistence —
// that is composed externally by the caller (typically package main).
type Catalog struct {
	themesDir string
}

// NewCatalog creates a Catalog for the given themes directory.
func NewCatalog(themesDir string) *Catalog {
	return &Catalog{themesDir: themesDir}
}

// Entries returns the ordered list of available themes with classification metadata.
func (c *Catalog) Entries() ([]ThemeInfo, error) {
	infos, err := ListOrdered(c.themesDir)
	if err != nil {
		return nil, fmt.Errorf("listing themes: %w", err)
	}
	return infos, nil
}

// Resolve loads a theme by name and returns the parsed Theme and whether it was found.
func (c *Catalog) Resolve(name string) (Theme, bool) {
	th, err := Load(name, c.themesDir)
	if err != nil {
		return Theme{}, false
	}
	return th, true
}
