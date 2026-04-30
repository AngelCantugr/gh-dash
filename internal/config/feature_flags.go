package config

import (
	"os"
	"strings"
)

const FF_REPO_VIEW = "FF_REPO_VIEW"

const FF_PROJECTS_VIEW = "FF_PROJECTS_VIEW"

const FF_MOCK_DATA = "FF_MOCK_DATA"

// defaultEnabled lists flags that are on by default.
// Users can still opt out by setting the env var to "0" or "false".
var defaultEnabled = map[string]bool{
	FF_PROJECTS_VIEW: true,
}

func IsFeatureEnabled(name string) bool {
	val, ok := os.LookupEnv(name)
	if !ok {
		return defaultEnabled[name]
	}
	lower := strings.ToLower(val)
	return lower != "0" && lower != "false"
}
