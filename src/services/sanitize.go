package services

import (
	"fmt"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

func SanitiseAndValidate(input string) (string, error) {
	policy := bluemonday.UGCPolicy().
		AllowElements("img").
		AllowAttrs("src", "alt").OnElements("img").
		AllowElements("math", "span").
		AllowAttrs("class").OnElements("span") // optional: for MathJax rendering

	sanitised := policy.Sanitize(input)

	if strings.TrimSpace(sanitised) == "" {
		return "", fmt.Errorf("input is empty or unsafe")
	}
	return sanitised, nil
}
