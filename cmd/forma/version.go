package main

import (
	"runtime/debug"
	"strings"
)

// versionOverride is set by release builds with -ldflags. Tagged module builds
// do not need it because Go records the module version in build information.
var versionOverride string

func currentVersion() string {
	if version := strings.TrimSpace(versionOverride); version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	return versionFromBuildInfo(info.Main.Version, info.Settings)
}

func versionFromBuildInfo(mainVersion string, settings []debug.BuildSetting) string {
	version := strings.TrimSpace(mainVersion)
	if version == "" || version == "(devel)" {
		version = "devel"
	}
	revision := ""
	modified := false
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	pseudoRevision, isPseudo := pseudoVersionRevision(version)
	if version != "devel" && !isPseudo && !modified {
		return version
	}
	if len(revision) < 7 {
		revision = pseudoRevision
	}

	result := "devel"
	if len(revision) >= 7 {
		result += " " + revision[:7]
	}
	if modified {
		result += " dirty"
	}
	return result
}

// pseudoVersionRevision recognizes and returns the timestamp-and-revision
// portion shared by Go pseudo-version forms, including forms based on a
// previous tagged release.
func pseudoVersionRevision(version string) (string, bool) {
	for index := 0; index+22 <= len(version); index++ {
		if !allDecimal(version[index:index+14]) || version[index+14] != '-' {
			continue
		}
		end := index + 15
		for end < len(version) && isHexDigit(version[end]) {
			end++
		}
		if end-(index+15) >= 7 {
			return version[index+15 : end], true
		}
	}
	return "", false
}

func allDecimal(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}

func isHexDigit(character byte) bool {
	return (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')
}
