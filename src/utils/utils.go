package utils

import (
	"log"
	"os"
	"unicode"
	"time"
)

func MustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Missing required environment variable: %s", key)
	}
	return val
}

func LexicalCardIDLess(a, b string) bool {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for k := 0; k < minLen; k++ {
		ar, br := rune(a[k]), rune(b[k])
		aIsLetter := unicode.IsLetter(ar)
		bIsLetter := unicode.IsLetter(br)
		if aIsLetter != bIsLetter {
			return aIsLetter // letters before non-letters
		}
		if ar != br {
			return ar < br
		}
	}
	return len(a) < len(b)
}

// Converts a Unix timestamp (seconds) to UK local time (Europe/London)
func UnixToUKTime(unixSeconds int64) time.Time {
    loc, err := time.LoadLocation("Europe/London")
    if err != nil {
        // Fallback to UTC if the location can't be loaded
        return time.Unix(unixSeconds, 0).UTC()
    }
    return time.Unix(unixSeconds, 0).In(loc)
}
