// Package utils provides utility functions for Kiro Gateway.
package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

// GenerateToolCallID generates a unique tool call ID (OpenAI format)
func GenerateToolCallID() string {
	return "call_" + uuid.New().String()[:24]
}

// GenerateToolUseID generates a unique tool use ID (Anthropic format)
func GenerateToolUseID() string {
	return "toolu_" + uuid.New().String()[:24]
}

// GenerateConversationID generates a unique conversation ID
func GenerateConversationID() string {
	return uuid.New().String()
}

// GetMachineFingerprint returns a unique machine fingerprint
func GetMachineFingerprint() string {
	hostname, _ := os.Hostname()
	username := "unknown"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	data := fmt.Sprintf("%s-%s-%s", hostname, username, runtime.GOOS)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// GetKiroHeaders returns headers for Kiro API requests
func GetKiroHeaders(accessToken string) map[string]string {
	return map[string]string{
		"Authorization":     "Bearer " + accessToken,
		"Content-Type":      "application/json",
		"User-Agent":        fmt.Sprintf("KiroGateway-Go/2.3 (%s; %s)", runtime.GOOS, runtime.GOARCH),
		"Accept":            "application/json, text/event-stream",
		"X-Amz-User-Agent":  "KiroGateway-Go/2.3",
	}
}

// ExtractTextContent extracts text from various content formats
func ExtractTextContent(content interface{}) string {
	if content == nil {
		return ""
	}

	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return fmt.Sprintf("%v", content)
	}
}

// SanitizeJSONSchema removes fields that Kiro API doesn't accept
func SanitizeJSONSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})

	for key, value := range schema {
		// Skip empty required arrays
		if key == "required" {
			if arr, ok := value.([]interface{}); ok && len(arr) == 0 {
				continue
			}
		}

		// Skip additionalProperties
		if key == "additionalProperties" {
			continue
		}

		// Recursively process nested objects
		switch v := value.(type) {
		case map[string]interface{}:
			if key == "properties" {
				props := make(map[string]interface{})
				for propKey, propValue := range v {
					if propMap, ok := propValue.(map[string]interface{}); ok {
						props[propKey] = SanitizeJSONSchema(propMap)
					} else {
						props[propKey] = propValue
					}
				}
				result[key] = props
			} else {
				result[key] = SanitizeJSONSchema(v)
			}
		case []interface{}:
			var newArr []interface{}
			for _, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					newArr = append(newArr, SanitizeJSONSchema(itemMap))
				} else {
					newArr = append(newArr, item)
				}
			}
			result[key] = newArr
		default:
			result[key] = value
		}
	}

	return result
}

// MustMarshal marshals to JSON or panics
func MustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// MustMarshalIndent marshals to indented JSON or panics
func MustMarshalIndent(v interface{}) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return b
}

// Contains checks if a string is in a slice
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// MapKeys returns the keys of a map
func MapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
