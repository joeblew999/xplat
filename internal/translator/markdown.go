package translator

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// MarkdownDoc represents a parsed markdown document
type MarkdownDoc struct {
	FrontMatter    map[string]interface{}
	Body           string
	rawFrontMatter string
}

// ParseMarkdown parses a markdown file with YAML front matter
func ParseMarkdown(content []byte) (*MarkdownDoc, error) {
	contentStr := string(content)

	// Check if file has front matter
	if !strings.HasPrefix(contentStr, "---") {
		// No front matter, entire content is body
		return &MarkdownDoc{
			FrontMatter: make(map[string]interface{}),
			Body:        contentStr,
		}, nil
	}

	// Find the end of front matter
	parts := strings.SplitN(contentStr, "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid front matter format")
	}

	frontMatterStr := parts[1]
	body := strings.TrimLeft(parts[2], "\n")

	// Parse YAML front matter
	var frontMatter map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontMatterStr), &frontMatter); err != nil {
		return nil, fmt.Errorf("failed to parse front matter: %w", err)
	}

	return &MarkdownDoc{
		FrontMatter:    frontMatter,
		Body:           body,
		rawFrontMatter: frontMatterStr,
	}, nil
}

// Reconstruct rebuilds the markdown file with front matter and body
func (md *MarkdownDoc) Reconstruct() (string, error) {
	var buf bytes.Buffer

	// Write front matter if it exists
	if len(md.FrontMatter) > 0 {
		buf.WriteString("---\n")

		// Marshal front matter to YAML
		yamlData, err := yaml.Marshal(md.FrontMatter)
		if err != nil {
			return "", fmt.Errorf("failed to marshal front matter: %w", err)
		}

		buf.Write(yamlData)
		buf.WriteString("---\n\n")
	}

	// Write body
	buf.WriteString(md.Body)

	return buf.String(), nil
}

// ParseI18n parses an i18n YAML file
func ParseI18n(content []byte) (map[string]string, error) {
	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse i18n YAML: %w", err)
	}

	// Convert to map[string]string
	result := make(map[string]string)
	for key, value := range data {
		if strVal, ok := value.(string); ok {
			result[key] = strVal
		} else {
			// Handle nested structures if needed
			result[key] = fmt.Sprintf("%v", value)
		}
	}

	return result, nil
}

// ReconstructI18n rebuilds an i18n YAML file
func ReconstructI18n(data map[string]string) (string, error) {
	// Convert map[string]string to map[string]interface{} for YAML marshaling
	yamlData := make(map[string]interface{})
	for key, value := range data {
		yamlData[key] = value
	}

	output, err := yaml.Marshal(yamlData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal i18n data: %w", err)
	}

	return string(output), nil
}
