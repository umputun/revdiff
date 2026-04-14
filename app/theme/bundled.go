package theme

import (
	"bytes"
	"fmt"
	"io/fs"
	"sort"
	"sync"

	"github.com/umputun/revdiff/themes"
)

const galleryDir = "gallery"

var galleryLoad struct {
	once   sync.Once
	themes map[string]Theme
	err    error
}

// Gallery returns all themes from the embedded gallery, keyed by filename.
// Results are cached after the first call since the embedded FS is immutable.
// Each call returns a deep copy so callers can mutate the result safely.
func Gallery() (map[string]Theme, error) {
	galleryLoad.once.Do(func() {
		galleryLoad.themes, galleryLoad.err = loadGallery()
	})
	if galleryLoad.err != nil {
		return nil, galleryLoad.err
	}
	return cloneGallery(galleryLoad.themes), nil
}

// loadGallery parses all theme files from the embedded gallery.
func loadGallery() (map[string]Theme, error) {
	entries, err := fs.ReadDir(themes.FS, galleryDir)
	if err != nil {
		return nil, fmt.Errorf("reading gallery: %w", err)
	}

	result := make(map[string]Theme, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(themes.FS, galleryDir+"/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading gallery theme %q: %w", e.Name(), err)
		}
		th, err := Parse(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("parsing gallery theme %q: %w", e.Name(), err)
		}
		result[e.Name()] = th
	}
	return result, nil
}

// GalleryNames returns sorted names of all themes in the embedded gallery.
func GalleryNames() ([]string, error) {
	gallery, err := Gallery()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(gallery))
	for name := range gallery {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// GalleryTheme returns a single theme from the embedded gallery by name.
func GalleryTheme(name string) (Theme, error) {
	gallery, err := Gallery()
	if err != nil {
		return Theme{}, err
	}
	th, ok := gallery[name]
	if !ok {
		return Theme{}, fmt.Errorf("gallery theme %q not found", name)
	}
	return th, nil
}

func cloneGallery(src map[string]Theme) map[string]Theme {
	dst := make(map[string]Theme, len(src))
	for name, th := range src {
		dst[name] = th.Clone()
	}
	return dst
}
