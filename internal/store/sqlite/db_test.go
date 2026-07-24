package sqlite

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterAvailableBeforeStart(t *testing.T) {
	// The Writer must be non-nil immediately after Open, before StartWriter is called.
	// Stores capture d.Writer() at construction time (in app.New), but StartWriter
	// is only called later (in app.Start). A nil Writer causes a panic on first write.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	db, err := Open(dbPath, logger)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	w := db.Writer()
	assert.NotNil(t, w, "Writer() must return a non-nil *Writer before StartWriter is called")
}
