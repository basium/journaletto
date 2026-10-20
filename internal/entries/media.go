package entries

import (
	"regexp"
	"strings"
)

var (
	fenceOpen   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	backticks   = regexp.MustCompile("`+")
	htmlImg     = regexp.MustCompile(`(?i)<img[\s/>]`)
	listItem    = regexp.MustCompile(`^ {0,3}([-*+]|\d{1,9}[.)])(\s|$)`)
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
	// An indented code block cannot interrupt a paragraph and, to stay conservative, is never
	// assumed inside a list (where indented text is item content, not code).
	prevBlank, inCode, inList := true, false, false
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
		if strings.TrimSpace(line) == "" {
			prevBlank = true
			prose = append(prose, "")
			continue
		}
		indented := strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
		switch {
		case indented && !inList && (prevBlank || inCode):
			inCode, prevBlank = true, false
			prose = append(prose, "")
			continue
		case listItem.MatchString(line):
			inList = true
		case !indented && prevBlank:
			inList = false
		}
		inCode, prevBlank = false, false
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
	return scanImages(s, defined)
}

// scanImages finds `![alt]` followed by `(` or `[`, or a shortcut reference to a defined label,
// tracking bracket nesting with a stack so alt text may contain brackets. Single pass, linear time.
func scanImages(s string, defined map[string]bool) bool {
	type open struct {
		at    int
		image bool
	}
	var stack []open
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '!':
			if i+1 < len(s) && s[i+1] == '[' {
				stack = append(stack, open{i + 2, true})
				i++
			}
		case '[':
			stack = append(stack, open{i + 1, false})
		case ']':
			if len(stack) == 0 {
				continue
			}
			o := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !o.image {
				continue
			}
			if i+1 < len(s) && (s[i+1] == '(' || s[i+1] == '[') {
				return true
			}
			if defined[strings.ToLower(strings.TrimSpace(s[o.at:i]))] {
				return true
			}
		}
	}
	return false
}
