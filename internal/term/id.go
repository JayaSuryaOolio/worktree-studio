package term

import (
	"crypto/rand"
	"encoding/hex"
)

// newSessionID returns a short random hex id, matching the id style used
// elsewhere in worktree-studio (internal/api.newID) without introducing a
// cross-package dependency for one helper.
func newSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
