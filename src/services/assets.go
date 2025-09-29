package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/abstract-tutoring/models"
)

var assetPattern = regexp.MustCompile(`asset://([a-zA-Z0-9/_\-\.]+)(?:\|([0-9]+x[0-9]+|[0-9]{1,3}%|))?(?:\|([a-z]+))?`)

func ResolveAssetsInContent(content string, assets []models.Asset, accessToken string) string {
	return assetPattern.ReplaceAllStringFunc(content, func(m string) string {
		match := assetPattern.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}

		assetID := match[1]
		sizeRaw := ""
		if len(match) >= 3 {
			sizeRaw = strings.TrimSpace(match[2])
		}
		align := ""
		if len(match) >= 4 {
			align = strings.TrimSpace(match[3])
		}

		for _, asset := range assets {
			if asset.ID == assetID && asset.Type == "image" {
				signedURL, err := GenerateSignedURL(accessToken, assetID)
				if err != nil {
					return `<em>Image unavailable</em>`
				}

				escapedAlt := template.HTMLEscapeString(asset.Alt)

				style := "max-width: 100%; height: auto;"
				display := ""

				if strings.HasSuffix(sizeRaw, "%") {
					style = fmt.Sprintf("width: %s; max-width: 100%%; height: auto;", sizeRaw)
				} else if strings.Contains(sizeRaw, "x") {
					parts := strings.Split(sizeRaw, "x")
					if len(parts) == 2 {
						w := parts[0]
						h := parts[1]
						style = fmt.Sprintf("width: %spx; height: %spx; max-width: 100%%;", w, h)
					}
				}

				switch align {
				case "left":
					style += " float: left;"
				case "right":
					style += " float: right;"
				case "center":
					display = "display: block;"
					style += " margin: 0 auto;"
				}

				return fmt.Sprintf(`<img src="%s" alt="%s" style="%s %s" />`, signedURL, escapedAlt, display, style)
			}
		}

		return m
	})
}

func GenerateSignedURL(accessToken, path string) (string, error) {
	apiURL := fmt.Sprintf(
		"%s/storage/v1/object/sign/flashcard-assets/%s",
		os.Getenv("NEXT_PUBLIC_SUPABASE_URL"),
		path,
	)

	expiry := 3600 // URL valid for 1 hour

	body := map[string]interface{}{
		"expiresIn": expiry,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", os.Getenv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to generate signed URL: status %d", resp.StatusCode)
	}

	var result struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return os.Getenv("NEXT_PUBLIC_SUPABASE_URL") + "/storage/v1" + result.SignedURL, nil
}
