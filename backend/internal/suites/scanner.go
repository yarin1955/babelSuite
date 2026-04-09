package suites

import "strings"

func scanLogicalStatements(source string) []string {
	lines := strings.Split(source, "\n")
	statements := make([]string, 0, len(lines))
	var current strings.Builder
	depthParen := 0
	depthBracket := 0
	depthBrace := 0

	flush := func() {
		statement := strings.TrimSpace(current.String())
		if statement != "" {
			statements = append(statements, statement)
		}
		current.Reset()
	}

	for _, rawLine := range lines {
		line := strings.TrimSpace(stripScannerComment(rawLine))
		if line == "" {
			continue
		}

		continued := strings.HasSuffix(line, "\\")
		if continued {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\\"))
		}
		if line == "" {
			continue
		}

		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(line)
		scanStructureDepths(line, &depthParen, &depthBracket, &depthBrace)
		if depthParen == 0 && depthBracket == 0 && depthBrace == 0 && !continued {
			flush()
		}
	}

	flush()
	return statements
}

func stripScannerComment(line string) string {
	inString := false
	escaped := false

	for index, ch := range line {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			continue
		}
		if ch == '#' {
			return line[:index]
		}
	}

	return line
}

func scanStructureDepths(line string, depthParen, depthBracket, depthBrace *int) {
	inString := false
	escaped := false

	for _, ch := range line {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '(':
			*depthParen = *depthParen + 1
		case ')':
			if *depthParen > 0 {
				*depthParen = *depthParen - 1
			}
		case '[':
			*depthBracket = *depthBracket + 1
		case ']':
			if *depthBracket > 0 {
				*depthBracket = *depthBracket - 1
			}
		case '{':
			*depthBrace = *depthBrace + 1
		case '}':
			if *depthBrace > 0 {
				*depthBrace = *depthBrace - 1
			}
		}
	}
}
