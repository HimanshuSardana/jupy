// Package highlight turns cell source into styled spans for the TUI. It is
// deliberately approximate: enough colour to read Python and Markdown, no
// error recovery beyond unterminated strings.
package highlight

import "strings"

// Style is a rendering class; the UI maps it to terminal attributes.
type Style int

const (
	Plain Style = iota
	Keyword
	String
	Comment
	Number
	Name
	Decorator
	Heading
	Emphasis
	Code
	Bullet
)

type Span struct {
	Text  string
	Style Style
}

// Lines splits source into logical lines of styled spans.
func Lines(source, kind string) [][]Span {
	switch kind {
	case "python":
		return pythonLines(source)
	case "markdown":
		return markdownLines(source)
	}
	return plainLines(source)
}

var pythonKeywords = map[string]bool{
	"and": true, "as": true, "assert": true, "async": true, "await": true,
	"break": true, "class": true, "continue": true, "def": true, "del": true,
	"elif": true, "else": true, "except": true, "False": true, "finally": true,
	"for": true, "from": true, "global": true, "if": true, "import": true,
	"in": true, "is": true, "lambda": true, "None": true, "nonlocal": true,
	"not": true, "or": true, "pass": true, "raise": true, "return": true,
	"True": true, "try": true, "while": true, "with": true, "yield": true,
	"match": true, "case": true,
}

func pythonLines(source string) [][]Span {
	logical := splitLines(source)
	out := make([][]Span, len(logical))
	triple := ""
	for i, line := range logical {
		out[i] = pythonLine(line, &triple)
	}
	return out
}

func pythonLine(line string, triple *string) []Span {
	var spans []Span
	add := func(text string, style Style) {
		if text != "" {
			spans = append(spans, Span{Text: text, Style: style})
		}
	}

	pendingDef := false
	for i := 0; i < len(line); {
		if *triple != "" {
			if j := strings.Index(line[i:], *triple); j >= 0 {
				add(line[i:i+j+len(*triple)], String)
				i += j + len(*triple)
				*triple = ""
				continue
			}
			add(line[i:], String)
			return spans
		}

		c := line[i]
		switch {
		case c == '#':
			add(line[i:], Comment)
			i = len(line)

		case c == '\'' || c == '"':
			quote := line[i : i+1]
			if strings.HasPrefix(line[i:], strings.Repeat(quote, 3)) {
				quote = strings.Repeat(quote, 3)
				if j := strings.Index(line[i+3:], quote); j >= 0 {
					add(line[i:i+3+j+3], String)
					i += 3 + j + 3
					continue
				}
				add(line[i:], String)
				*triple = quote
				i = len(line)
				continue
			}
			j := i + 1
			for j < len(line) {
				if line[j] == '\\' {
					j += 2
					continue
				}
				if line[j] == quote[0] {
					j++
					break
				}
				j++
			}
			if j > len(line) {
				j = len(line)
			}
			add(line[i:j], String)
			i = j

		case c >= '0' && c <= '9':
			j := i
			for j < len(line) && (isAlnum(line[j]) || line[j] == '.') {
				j++
			}
			add(line[i:j], Number)
			i = j

		case isIdentStart(c):
			j := i
			for j < len(line) && isAlnum(line[j]) {
				j++
			}
			word := line[i:j]
			switch {
			case word == "def" || word == "class":
				add(word, Keyword)
				pendingDef = true
			case pythonKeywords[word]:
				add(word, Keyword)
			case pendingDef:
				add(word, Name)
				pendingDef = false
			default:
				add(word, Plain)
			}
			i = j

		case c == '@' && !pendingDef:
			j := i + 1
			for j < len(line) && (isIdentStart(line[j]) || isAlnum(line[j]) || line[j] == '.') {
				j++
			}
			add(line[i:j], Decorator)
			i = j

		default:
			add(string(c), Plain)
			i++
		}
	}
	return spans
}

func markdownLines(source string) [][]Span {
	logical := splitLines(source)
	out := make([][]Span, len(logical))
	inFence := false
	for i, line := range logical {
		trimmed := strings.TrimLeft(line, " ")
		indent := line[:len(line)-len(trimmed)]

		if strings.HasPrefix(trimmed, "```") {
			out[i] = []Span{{Text: line, Style: Code}}
			inFence = !inFence
			continue
		}
		if inFence {
			out[i] = []Span{{Text: line, Style: Code}}
			continue
		}
		if strings.HasPrefix(trimmed, "#") && strings.HasPrefix(strings.TrimLeft(trimmed, "#"), " ") {
			out[i] = []Span{{Text: line, Style: Heading}}
			continue
		}
		if trimmed == "" {
			out[i] = nil
			continue
		}

		spans := []Span{}
		if indent != "" {
			spans = append(spans, Span{Text: indent, Style: Plain})
		}
		body := trimmed
		switch {
		case strings.HasPrefix(body, ">"):
			spans = append(spans, Span{Text: ">", Style: Bullet})
			body = body[1:]
		case strings.HasPrefix(body, "- "), strings.HasPrefix(body, "* "), strings.HasPrefix(body, "+ "):
			spans = append(spans, Span{Text: body[:1], Style: Bullet})
			body = body[1:]
		}
		out[i] = append(spans, inline(body)...)
	}
	return out
}

// inline styles `code`, **bold**, *italic* and [links](url) inside one line.
func inline(line string) []Span {
	var spans []Span
	add := func(text string, style Style) {
		if text != "" {
			spans = append(spans, Span{Text: text, Style: style})
		}
	}

	for i := 0; i < len(line); {
		switch {
		case line[i] == '`':
			if j := strings.IndexByte(line[i+1:], '`'); j >= 0 {
				add(line[i:i+j+2], Code)
				i += j + 2
				continue
			}
			add(line[i:], Plain)
			return spans

		case strings.HasPrefix(line[i:], "**"):
			if j := strings.Index(line[i+2:], "**"); j >= 0 {
				add(line[i:i+j+4], Emphasis)
				i += j + 4
				continue
			}
			add(line[i:], Plain)
			return spans

		case strings.HasPrefix(line[i:], "["):
			if j := strings.Index(line[i:], "]("); j >= 0 {
				if k := strings.IndexByte(line[i+j:], ')'); k >= 0 {
					add(line[i:i+j+1], Plain)
					add(line[i+j+1:i+j+k+1], Code)
					i += j + k + 1
					continue
				}
			}
			add(line[i:i+1], Plain)
			i++

		default:
			j := i
			for j < len(line) && line[j] != '`' && line[j] != '[' && !strings.HasPrefix(line[j:], "**") {
				j++
			}
			add(line[i:j], Plain)
			i = j
		}
	}
	return spans
}

func plainLines(source string) [][]Span {
	logical := splitLines(source)
	out := make([][]Span, len(logical))
	for i, line := range logical {
		out[i] = []Span{{Text: line, Style: Plain}}
	}
	return out
}

// splitLines keeps blank lines and drops the empty tail after a final newline.
func splitLines(source string) []string {
	lines := strings.Split(source, "\n")
	if n := len(lines); n > 1 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

func isIdentStart(c byte) bool {
	return c == '_' || (c|0x20 >= 'a' && c|0x20 <= 'z')
}

func isAlnum(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c|0x20 >= 'a' && c|0x20 <= 'z')
}
