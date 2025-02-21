package clngo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clngo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContent_Store(t *testing.T) {
	t.Parallel()

	t.Run("store new content", func(t *testing.T) {
		t.Parallel()
		store, err := clngo.NewStore(t.TempDir())
		require.NoError(t, err)

		content := clngo.NewContent(store)
		testHash := "abcdef123456"
		testData := []byte("test content")

		err = content.Store(testHash, testData)
		require.NoError(t, err)

		// Verify content was stored
		storedPath := filepath.Join(store.Path(), testHash)
		storedData, err := os.ReadFile(storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("store existing content", func(t *testing.T) {
		t.Parallel()
		store, err := clngo.NewStore(t.TempDir())
		require.NoError(t, err)

		content := clngo.NewContent(store)
		testHash := "abcdef123456"
		testData := []byte("test content")

		// Store content twice
		err = content.Store(testHash, testData)
		require.NoError(t, err)
		err = content.Store(testHash, []byte("different content"))
		require.NoError(t, err)

		// Verify original content remains
		storedPath := filepath.Join(store.Path(), testHash)
		storedData, err := os.ReadFile(storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})
}

func TestContent_Link(t *testing.T) {
	t.Parallel()

	t.Run("create new link", func(t *testing.T) {
		t.Parallel()
		storeDir := t.TempDir()
		store, err := clngo.NewStore(storeDir)
		require.NoError(t, err)

		content := clngo.NewContent(store)
		testHash := "abcdef123456"
		testData := []byte("test content")

		// First store some content
		err = content.Store(testHash, testData)
		require.NoError(t, err)

		// Then create a link to it
		targetDir := t.TempDir()
		targetPath := filepath.Join(targetDir, "subdir", "test.txt")
		err = content.Link(testHash, targetPath)
		require.NoError(t, err)

		// Verify link was created and contains correct content
		linkedData, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, testData, linkedData)

		// Verify it's a hard link by checking inode numbers
		sourceInfo, err := os.Stat(filepath.Join(store.Path(), testHash))
		require.NoError(t, err)
		targetInfo, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, sourceInfo.Sys(), targetInfo.Sys())
	})

	t.Run("link to existing file", func(t *testing.T) {
		t.Parallel()
		store, err := clngo.NewStore(t.TempDir())
		require.NoError(t, err)

		content := clngo.NewContent(store)
		testHash := "abcdef123456"
		testData := []byte("test content")

		// Store content
		err = content.Store(testHash, testData)
		require.NoError(t, err)

		// Create target file
		targetDir := t.TempDir()
		targetPath := filepath.Join(targetDir, "test.txt")
		err = os.WriteFile(targetPath, []byte("existing content"), 0644)
		require.NoError(t, err)

		// Try to create link
		err = content.Link(testHash, targetPath)
		require.NoError(t, err)

		// Verify original content remains
		existingData, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("existing content"), existingData)
	})
}
