package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

type SecretPattern struct {
	Name  string
	Regex *regexp.Regexp
}

func InitSecretPatterns() []SecretPattern {
	patterns := []struct {
		name    string
		pattern string
	}{
		{"AWS Access Key", `AKIA[0-9A-Z]{16}`},
		{"AWS Secret Key", `(?i)(?:aws_secret|secret_access_key|aws_secret_access_key)\s*[=:]\s*['"]?([0-9a-zA-Z/+=]{40})`},
		{"GitHub PAT", `ghp_[0-9a-zA-Z]{36}`},
		{"GitHub OAuth", `gho_[0-9a-zA-Z]{36}`},
		{"GitHub App Token", `ghs_[0-9a-zA-Z]{36}`},
		{"GitHub Classic Token", `ghp_[0-9a-zA-Z]{36}`},
		{"Slack Bot Token", `xoxb-[0-9a-zA-Z-]{10,}`},
		{"Slack User Token", `xoxp-[0-9a-zA-Z-]{10,}`},
		{"Slack Webhook", `https://hooks\.slack\.com/services/[A-Za-z0-9/]+`},
		{"OpenAI/Anthropic API Key", `sk-[a-zA-Z0-9_-]{20,}`},
		{"Private Key", `-----BEGIN[A-Z ]*PRIVATE KEY-----`},
		{"Password Assignment", `(?i)(?:password|passwd|pwd|secret)\s*[=:]\s*['"]?[^\s'"]{8,}`},
		{"Database Connection String", `(?:mongodb|postgres|postgresql|mysql|redis|amqp|mssql)://[^\s'"]{10,}`},
		{"Bearer Token", `(?i)bearer\s+[a-zA-Z0-9_\-.~+/]{20,}=*`},
		{"Generic API Key", `(?i)(?:api[_-]?key|apikey|api[_-]?secret)\s*[=:]\s*['"]?[a-zA-Z0-9_-]{16,}`},
		{"Hex Secret", `(?i)(?:token|secret|key)\s*[=:]\s*['"]?[0-9a-f]{32,}`},
		{"Stripe Key", `(?:sk|pk)_(?:test|live)_[0-9a-zA-Z]{10,}`},
		{"Twilio Token", `SK[0-9a-fA-F]{32}`},
		{"SendGrid Key", `SG\.[0-9a-zA-Z_-]{22}\.[0-9a-zA-Z_-]{43}`},
		{"Google API Key", `AIza[0-9A-Za-z_-]{35}`},
		{"SSH Private Key Content", `(?:OPENSSH|RSA|DSA|EC|PGP) PRIVATE KEY`},
	}

	var result []SecretPattern
	for _, p := range patterns {
		re, err := regexp.Compile(p.pattern)
		if err != nil {
			continue
		}
		result = append(result, SecretPattern{Name: p.name, Regex: re})
	}
	return result
}

var quickFilterKeywords = []string{
	"AKIA", "ghp_", "gho_", "ghs_", "xoxb-", "xoxp-", "sk-", "sk_test_", "sk_live_",
	"pk_test_", "pk_live_", "-----BEGIN", "PRIVATE KEY",
	"password", "PASSWORD", "passwd", "PASSWD", "secret", "SECRET",
	"mongodb://", "postgres://", "postgresql://", "mysql://", "redis://", "amqp://", "mssql://",
	"bearer", "Bearer", "BEARER",
	"api_key", "API_KEY", "apikey", "APIKEY", "api-key", "API-KEY", "api_secret", "API_SECRET",
	"hooks.slack.com",
	"SG.", "AIza",
	"token=", "TOKEN=", "token:", "TOKEN:",
}

func quickFilter(text string) bool {
	for _, kw := range quickFilterKeywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func ScanForSecrets(inv *FileInventory) ([]SecretFinding, error) {
	patterns := InitSecretPatterns()
	var findings []SecretFinding
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for _, convFile := range inv.ConversationFiles {
		wg.Add(1)
		sem <- struct{}{}
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()

			proj := projectFromConvPath(path)
			seen := make(map[string]bool)
			var localFindings []SecretFinding

			_ = ParseConversationFileStreaming(path, func(cl ConversationLine) error {
				if cl.Type != "user" && cl.Type != "assistant" {
					return nil
				}
				if cl.Message == nil {
					return nil
				}

				text := extractTextContent(cl.Message.Content)
				if text == "" || !quickFilter(text) {
					return nil
				}

				ts := timestampToDateStr(cl.Timestamp)

				for _, p := range patterns {
					matches := p.Regex.FindAllString(text, 5)
					for _, m := range matches {
						redacted := RedactMatch(m)
						dedup := p.Name + "|" + redacted
						if seen[dedup] {
							continue
						}
						seen[dedup] = true
						localFindings = append(localFindings, SecretFinding{
							ConversationFile: path,
							SessionID:        cl.SessionID,
							Project:          DecodeProjectPath(proj),
							PatternName:      p.Name,
							Match:            redacted,
							RawMatch:         m,
							Timestamp:        ts,
						})
					}
				}
				return nil
			})

			if len(localFindings) > 0 {
				mu.Lock()
				findings = append(findings, localFindings...)
				mu.Unlock()
			}
		}(convFile)
	}
	wg.Wait()

	return findings, nil
}

func RedactMatch(match string) string {
	if len(match) <= 8 {
		return match[:1] + strings.Repeat("*", len(match)-1)
	}
	show := 4
	if len(match) < 16 {
		show = 2
	}
	return match[:show] + strings.Repeat("*", len(match)-2*show) + match[len(match)-show:]
}

func OverwriteSecrets(findings []SecretFinding) (int, int, error) {
	byFile := make(map[string][]SecretFinding)
	for _, f := range findings {
		byFile[f.ConversationFile] = append(byFile[f.ConversationFile], f)
	}

	filesModified := 0
	secretsCensored := 0

	for path, filefindings := range byFile {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: cannot read %s: %v\n", path, err)
			continue
		}

		original := string(data)
		modified := original

		for _, f := range filefindings {
			if f.RawMatch == "" {
				continue
			}
			censored := "***CENSORED:" + f.PatternName + "***"
			before := modified
			modified = strings.ReplaceAll(modified, f.RawMatch, censored)
			if modified != before {
				secretsCensored++
			}
		}

		if modified == original {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: cannot stat %s: %v\n", path, err)
			continue
		}

		if err := os.WriteFile(path, []byte(modified), info.Mode()); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: cannot write %s: %v\n", path, err)
			continue
		}
		filesModified++
	}

	return filesModified, secretsCensored, nil
}
