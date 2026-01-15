// Package syncgh provides GitHub sync operations without requiring the gh CLI.
package syncgh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/joeblew999/xplat/internal/config"
)

// PollState tracks commit hashes for polling comparison.
// This enables the Poller to detect changes and trigger cache invalidation.
type PollState struct {
	// Repos maps "owner/repo" to commit hash info
	Repos map[string]RepoCommitState `json:"repos"`

	// UpdatedAt is when the state was last saved
	UpdatedAt time.Time `json:"updated_at"`
}

// RepoCommitState holds the last known commit for a repo+ref
type RepoCommitState struct {
	// Ref is the branch or tag being tracked (e.g., "main", "v1.2.3")
	Ref string `json:"ref"`

	// CommitHash is the last known commit hash (short, 8 chars)
	CommitHash string `json:"commit_hash"`

	// LastChecked is when this repo was last polled
	LastChecked time.Time `json:"last_checked"`
}

// pollStateFile is the filename for poll state persistence
const pollStateFile = "syncgh-poll-state.json"

// pollStateMutex protects concurrent access to the state file
var pollStateMutex sync.Mutex

// LoadPollState loads the poll state from disk.
// Returns empty state if file doesn't exist.
func LoadPollState() (*PollState, error) {
	pollStateMutex.Lock()
	defer pollStateMutex.Unlock()

	statePath := filepath.Join(config.XplatCache(), pollStateFile)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty state if file doesn't exist
			return &PollState{
				Repos: make(map[string]RepoCommitState),
			}, nil
		}
		return nil, err
	}

	var state PollState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.Repos == nil {
		state.Repos = make(map[string]RepoCommitState)
	}

	return &state, nil
}

// SavePollState saves the poll state to disk.
func SavePollState(state *PollState) error {
	pollStateMutex.Lock()
	defer pollStateMutex.Unlock()

	cacheDir := config.XplatCache()
	if err := os.MkdirAll(cacheDir, config.DefaultDirPerms); err != nil {
		return err
	}

	state.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	statePath := filepath.Join(cacheDir, pollStateFile)
	return os.WriteFile(statePath, data, config.DefaultFilePerms)
}

// GetRepoHash returns the last known commit hash for a repo.
// Returns empty string if repo not found.
func (s *PollState) GetRepoHash(repo, ref string) string {
	key := makeRepoKey(repo, ref)
	if state, ok := s.Repos[key]; ok {
		return state.CommitHash
	}
	return ""
}

// SetRepoHash updates the commit hash for a repo.
func (s *PollState) SetRepoHash(repo, ref, hash string) {
	key := makeRepoKey(repo, ref)
	s.Repos[key] = RepoCommitState{
		Ref:         ref,
		CommitHash:  hash,
		LastChecked: time.Now().UTC(),
	}
}

// HasChanged returns true if the new hash differs from stored hash.
func (s *PollState) HasChanged(repo, ref, newHash string) bool {
	oldHash := s.GetRepoHash(repo, ref)
	if oldHash == "" {
		// No previous state - treat as changed (first poll)
		return true
	}
	return oldHash != newHash
}

// makeRepoKey creates a unique key for repo+ref
func makeRepoKey(repo, ref string) string {
	return repo + "@" + ref
}

// StatefulPoller wraps Poller with state persistence.
// It tracks commit hashes between polls and only triggers callbacks on actual changes.
type StatefulPoller struct {
	*Poller
	state    *PollState
	onChange func(repo, ref, oldHash, newHash string)
}

// NewStatefulPoller creates a poller that tracks state.
func NewStatefulPoller(interval time.Duration, repos []RepoConfig, token string) (*StatefulPoller, error) {
	state, err := LoadPollState()
	if err != nil {
		return nil, err
	}

	sp := &StatefulPoller{
		Poller: NewPoller(interval, repos, token),
		state:  state,
	}

	// Wire up the internal callback to check state
	sp.Poller.OnUpdate(func(subsystem, _, newHash string) {
		// Determine ref from config
		ref := "main"
		for _, r := range repos {
			if r.Subsystem == subsystem {
				if r.UseTag {
					ref = r.Tag
				} else if r.Branch != "" {
					ref = r.Branch
				}
				break
			}
		}

		// Check if actually changed
		if sp.state.HasChanged(subsystem, ref, newHash) {
			oldHash := sp.state.GetRepoHash(subsystem, ref)

			// Update state
			sp.state.SetRepoHash(subsystem, ref, newHash)

			// Save state
			if err := SavePollState(sp.state); err != nil {
				// Log but don't fail
				println("syncgh: Failed to save poll state:", err.Error())
			}

			// Trigger callback if set
			if sp.onChange != nil {
				sp.onChange(subsystem, ref, oldHash, newHash)
			}
		}
	})

	return sp, nil
}

// OnChange sets the callback for when a repo actually changes.
// Unlike OnUpdate, this is only called when the commit hash differs from previous poll.
func (sp *StatefulPoller) OnChange(callback func(repo, ref, oldHash, newHash string)) {
	sp.onChange = callback
}

// State returns the current poll state (for inspection)
func (sp *StatefulPoller) State() *PollState {
	return sp.state
}
