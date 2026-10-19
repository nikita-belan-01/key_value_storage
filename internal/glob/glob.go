package glob

func Match(pattern, name string) bool {
	p, n := 0, 0
	starP, starN := -1, 0

	for n < len(name) {
		if p < len(pattern) && pattern[p] != '*' {
			ok, i := matchToken(pattern, p, name[n])
			if ok {
				p += i
				n++

				continue
			}
		}
		if p < len(pattern) && pattern[p] == '*' {
			starP, starN = p, n
			p++

			continue
		}
		if starP != -1 {
			p = starP + 1
			starN++
			n = starN

			continue
		}

		return false
	}

	for p < len(pattern) && pattern[p] == '*' {
		p++
	}

	return p == len(pattern)
}

func LiteralPrefix(pattern string) string {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*', '?', '[', '\\':
			return pattern[:i]
		}
	}

	return pattern
}

func matchToken(pattern string, p int, b byte) (bool, int) {
	switch pattern[p] {
	case '?':
		return true, 1
	case '\\':
		if p+1 >= len(pattern) {
			return b == '\\', 1
		}
		return b == pattern[p+1], 2
	case '[':
		return matchClass(pattern, p, b)
	default:
		return b == pattern[p], 1
	}
}

func matchClass(pattern string, p int, b byte) (bool, int) {
	i := p + 1
	negate := false

	if i < len(pattern) && pattern[i] == '^' { // check [^ (negate)
		negate = true
		i++
	}

	matched := false

	for i < len(pattern) && pattern[i] != ']' { // while not a symbol ]
		switch {
		case pattern[i] == '\\' && i+1 < len(pattern): // skip escape
			if b == pattern[i+1] {
				matched = true
			}
			i += 2

		case i+2 < len(pattern) && pattern[i+1] == '-' && pattern[i+2] != ']': // a-z | z-a | 0-9 | 9-0
			lo, hi := pattern[i], pattern[i+2]
			if lo > hi {
				lo, hi = hi, lo // z-a to a-z or 9-0 to 0-9
			}

			if lo <= b && b <= hi {
				matched = true
			}
			i += 3
		default: // '*', '?', '['
			if b == pattern[i] {
				matched = true
			}
			i++
		}
	}

	if negate {
		matched = !matched
	}

	if i >= len(pattern) { // end pattern
		return matched, len(pattern) - p
	}

	return matched, i - p + 1 // i points at ']'
}
