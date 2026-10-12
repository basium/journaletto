package journal

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Render builds the full raw Markdown file content for an entry: YAML front
// matter delimited by "---" lines, followed by the title as a top-level
// heading and the user's Markdown description verbatim (JE-002, JE-004).
func Render(fm FrontMatter, title, description string) (string, error) {
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshaling front matter: %w", err)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(yamlBytes) // yaml.Marshal output already ends with "\n"
	b.WriteString("---\n\n")
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(description)
	b.WriteString("\n")

	return b.String(), nil
}
