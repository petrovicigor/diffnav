package diffviewer

import (
	"strings"
)

// delta's --side-by-side has no negation, so a gitconfig with
// delta.side-by-side=true makes every render side-by-side: a new or deleted
// file got an empty half, and the unified toggle did nothing.
//
// A unified render therefore runs delta with delta.side-by-side = false in
// GIT_CONFIG_PARAMETERS, the variable `git -c` uses. delta reads it on top of
// the gitconfig it would read anyway (system, global, the repo's own), so a
// unified render sees exactly the config a side-by-side one does, plus that
// one value; --side-by-side on the command line still wins over it.
// (It used to point --config at a temp file including the global gitconfig,
// which dropped the system and repo configs and left a file in $TMPDIR.)

const (
	gitConfigParamsVar = "GIT_CONFIG_PARAMETERS"
	unifiedConfigParam = "'delta.side-by-side'='false'"
)

// deltaEnv is the environment for a delta render: environ (the process's
// own) as is for side-by-side, and for a unified render with
// unifiedConfigParam appended to any GIT_CONFIG_PARAMETERS already set (git
// sets it when diffnav runs as a pager under `git -c …`); the later value
// wins in delta.
func deltaEnv(environ []string, sideBySide bool) []string {
	if sideBySide {
		return environ
	}
	env := make([]string, 0, len(environ)+1)
	params := unifiedConfigParam
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, gitConfigParamsVar+"="); ok {
			if v != "" {
				params = v + " " + unifiedConfigParam
			}
			continue
		}
		env = append(env, kv)
	}
	return append(env, gitConfigParamsVar+"="+params)
}
