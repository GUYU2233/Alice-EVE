package app

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var chatLogFilenamePattern = regexp.MustCompile(`^(.+)_(\d{8})_(\d{6})_(\d+)\.txt$`)
var gameLogFilenamePattern = regexp.MustCompile(`^(\d{8})_(\d{6})_(\d+)\.txt$`)

type eveLogFileMetadata struct {
	Path        string
	Channel     string
	CharacterID int64
	Session     time.Time
}

func matchChatLogFilename(path string) (eveLogFileMetadata, bool) {
	name := filepath.Base(path)
	match := chatLogFilenamePattern.FindStringSubmatch(name)
	if len(match) != 5 {
		return eveLogFileMetadata{}, false
	}
	session, err := time.ParseInLocation("20060102 150405", match[2]+" "+match[3], time.UTC)
	if err != nil {
		return eveLogFileMetadata{}, false
	}
	characterID, err := strconv.ParseInt(match[4], 10, 64)
	if err != nil || characterID <= 0 {
		return eveLogFileMetadata{}, false
	}
	channel := strings.TrimSpace(match[1])
	if channel == "" {
		return eveLogFileMetadata{}, false
	}
	return eveLogFileMetadata{Path: path, Channel: channel, CharacterID: characterID, Session: session}, true
}

func matchGameLogFilename(path string) (eveLogFileMetadata, bool) {
	name := filepath.Base(path)
	match := gameLogFilenamePattern.FindStringSubmatch(name)
	if len(match) != 4 {
		return eveLogFileMetadata{}, false
	}
	session, err := time.ParseInLocation("20060102 150405", match[1]+" "+match[2], time.UTC)
	if err != nil {
		return eveLogFileMetadata{}, false
	}
	characterID, err := strconv.ParseInt(match[3], 10, 64)
	if err != nil || characterID <= 0 {
		return eveLogFileMetadata{}, false
	}
	return eveLogFileMetadata{Path: path, CharacterID: characterID, Session: session}, true
}
