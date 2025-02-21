package clngo

import (
	"strings"
)

// TreeEntry represents a single entry in a git tree
type TreeEntry struct {
	Mode string // File mode (e.g., "100644" for regular file)
	Type string // Object type (blob or tree)
	Hash string // Object hash
	Path string // File path
}

// Tree represents a git tree object with its entries
type Tree struct {
	entries []TreeEntry
	path    string
}

// Entries returns the tree entries
func (t *Tree) Entries() []TreeEntry {
	return t.entries
}

// Path returns the tree path
func (t *Tree) Path() string {
	return t.path
}

// ParseTreeEntry parses a single line from git ls-tree output
func ParseTreeEntry(line string) (TreeEntry, error) {
	// Format: <mode> <type> <hash> <path>
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return TreeEntry{}, &WrappedError{
			Op:      "parse_tree_entry",
			Context: "invalid tree entry format",
			Err:     ErrParseTree,
		}
	}

	return TreeEntry{
		Mode: parts[0],
		Type: parts[1],
		Hash: parts[2],
		Path: strings.Join(parts[3:], " "), // Handle paths with spaces
	}, nil
}

// ParseTree parses the complete output of git ls-tree
func ParseTree(output, path string) (*Tree, error) {
	if output == "" {
		return &Tree{
			entries: make([]TreeEntry, 0),
			path:    path,
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	entries := make([]TreeEntry, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		entry, err := ParseTreeEntry(line)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return &Tree{
		entries: entries,
		path:    path,
	}, nil
}
