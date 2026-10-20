package entries

import (
	"regexp"
	"strings"
)

var (
	fenceOpen   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	backticks   = regexp.MustCompile("`+")
	htmlImg     = regexp.MustCompile(`(?i)<img[\s/>]`)
	mdImage     = regexp.MustCompile(`!\[([^\]]*)\](\(|\[)?`)
	definitions = regexp.MustCompile(`(?m)^ {0,3}\[([^\]]+)\]:`)
)

// stripCodeSpans removes `code` spans: a backtick run closes at the next run of equal length.
func stripCodeSpans(line string) string {
	runs := backticks.FindAllStringIndex(line, -1)
	var b strings.Builder
	pos := 0
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			if runs[j][1]-runs[j][0] == runs[i][1]-runs[i][0] {
				b.WriteString(line[pos:runs[i][0]])
				pos = runs[j][1]
				i = j
				break
			}
		}
	}
	b.WriteString(line[pos:])
	return b.String()
}

// hasEmbeddedImage reports whether Markdown text embeds an image (inline, reference-style or
// HTML <img>). Fenced code blocks and inline code spans are literal text and are ignored.
func hasEmbeddedImage(text string) bool {
	var prose []string
	fence := "" // active fence marker, "" when outside a fenced block
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if fence != "" {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
			continue
		}
		if m := fenceOpen.FindStringSubmatch(line); m != nil {
			fence = m[1]
			continue
		}
		prose = append(prose, stripCodeSpans(line))
	}
	s := strings.Join(prose, "\n")
	if htmlImg.MatchString(s) {
		return true
	}
	defined := map[string]bool{}
	for _, d := range definitions.FindAllStringSubmatch(s, -1) {
		defined[strings.ToLower(strings.TrimSpace(d[1]))] = true
	}
	for _, m := range mdImage.FindAllStringSubmatch(s, -1) {
		if m[2] != "" || defined[strings.ToLower(strings.TrimSpace(m[1]))] {
			return true
		}
	}
	return false
}
