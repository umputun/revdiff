package diff

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
)

const fileFingerprintVersion = "revdiff-file-fingerprint-v1"

// FileFingerprint returns a stable identity for the effective diff of one file.
// It intentionally ignores line numbers, context lines, and compact-mode dividers
// when the file has additions or removals. This keeps the identity stable when a
// rebase merely shifts a hunk or changes unrelated surrounding context.
//
// Context-only sources have no add/remove rows, so all non-divider content is
// included. File status and rename origin are included to distinguish metadata-only
// changes such as pure renames.
func FileFingerprint(entry FileEntry, lines []DiffLine) string {
	h := sha256.New()
	writeFingerprintField(h, fileFingerprintVersion)
	writeFingerprintField(h, entry.Path)
	writeFingerprintField(h, entry.OldPath)
	writeFingerprintField(h, string(entry.Status))

	hasChanges := false
	for _, line := range lines {
		if line.ChangeType == ChangeAdd || line.ChangeType == ChangeRemove {
			hasChanges = true
			break
		}
	}

	for _, line := range lines {
		if line.ChangeType == ChangeDivider {
			continue
		}
		if hasChanges && line.ChangeType != ChangeAdd && line.ChangeType != ChangeRemove {
			continue
		}
		writeFingerprintField(h, string(line.ChangeType))
		writeFingerprintField(h, line.Content)
		if line.IsBinary {
			writeFingerprintField(h, "binary")
		}
		if line.IsPlaceholder {
			writeFingerprintField(h, "placeholder")
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// ReviewFingerprintStable reports whether the rendered diff exposes enough
// content to compare fingerprints across reloads. Binary and placeholder rows
// are opaque: their display text may stay identical while the underlying file
// changes, so their reviewed mark is conservatively cleared on reload.
func ReviewFingerprintStable(lines []DiffLine) bool {
	for _, line := range lines {
		if line.IsBinary || line.IsPlaceholder {
			return false
		}
	}
	return true
}

// writeFingerprintField uses length-prefix framing so distinct field sequences
// cannot collide through delimiter-like file content.
func writeFingerprintField(h hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(value))
}
