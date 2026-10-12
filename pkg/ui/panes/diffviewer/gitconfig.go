package diffviewer

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"charm.land/log/v2"
)

// delta's --side-by-side has no negation, so a gitconfig with
// delta.side-by-side=true makes every Miasma render side-by-side: a new or
// deleted file got an empty half, and the unified toggle did nothing.
// A unified render therefore runs delta with --config pointing at
// unifiedGitconfig: the user's global gitconfig (features included) followed
// by side-by-side = false, which wins as the later value.

var (
	unifiedConfigOnce sync.Once
	unifiedConfigPath string
)

// unifiedGitconfigText includes the global gitconfig files, in git's order
// (XDG, then ~/.gitconfig; a missing one is skipped), then turns
// side-by-side off.
func unifiedGitconfigText(home, xdgConfigHome string) string {
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(home, ".config")
	}
	return fmt.Sprintf(
		"[include]\n\tpath = %s\n\tpath = %s\n[delta]\n\tside-by-side = false\n",
		filepath.Join(xdgConfigHome, "git", "config"),
		filepath.Join(home, ".gitconfig"),
	)
}

// unifiedGitconfig writes the unified config once per process and returns
// its path, or "" when it can't (delta then falls back to the gitconfig's
// own side-by-side setting). The file is named by its content, so every run
// with the same home shares one file and nothing piles up in the temp dir
// however diffnav exits; it is replaced atomically, so parallel runs never
// read half a file.
func unifiedGitconfig() string {
	unifiedConfigOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Warn("unified delta config: no home dir", "err", err)
			return
		}
		text := unifiedGitconfigText(home, os.Getenv("XDG_CONFIG_HOME"))
		path := unifiedGitconfigPath(os.TempDir(), text)
		if cur, err := os.ReadFile(path); err == nil && string(cur) == text {
			unifiedConfigPath = path
			return
		}
		f, err := os.CreateTemp(filepath.Dir(path), ".diffnav-unified-*")
		if err != nil {
			log.Warn("unified delta config", "err", err)
			return
		}
		_, err = f.WriteString(text)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Rename(f.Name(), path)
		}
		if err != nil {
			log.Warn("unified delta config", "err", err)
			os.Remove(f.Name())
			return
		}
		unifiedConfigPath = path
	})
	return unifiedConfigPath
}

// unifiedGitconfigPath is where text lives in dir: named by its hash.
func unifiedGitconfigPath(dir, text string) string {
	sum := sha256.Sum256([]byte(text))
	return filepath.Join(dir, fmt.Sprintf("diffnav-unified-%x.gitconfig", sum[:6]))
}
