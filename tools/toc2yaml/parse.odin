package toc2yaml

import "core:fmt"
import "core:strconv"
import "core:strings"
import "core:unicode/utf8"

EN_DASH :: "–"

Item :: struct {
	is_range: bool,
	number:   int,
	end:      int,
	name:     string,
	page:     int,
	has_page: bool,
	unit:     int,
}

parse_toc :: proc(text, curriculum_name: string) -> (items: []Item, err: string) {
	_ = curriculum_name
	src := strings.trim_prefix(text, "\uFEFF")
	src, _ = strings.replace_all(src, "\r\n", "\n")
	src, _ = strings.replace_all(src, "\r", "\n")
	lines := strings.split(src, "\n")

	out: [dynamic]Item
	cur: Item
	open := false
	unit := 0

	for raw in lines {
		line := strings.trim_space(raw)
		if line == "" {
			continue
		}

		if n, ok := parse_unit_line(line); ok {
			flush_item(&out, &cur, &open)
			unit = n
			continue
		}

		if start, rest, ok := parse_lesson_start(line); ok {
			flush_item(&out, &cur, &open)
			cur = start
			cur.unit = unit
			open = true
			apply_fragment(&cur, rest)
			continue
		}

		if open {
			apply_fragment(&cur, line)
			continue
		}
	}
	flush_item(&out, &cur, &open)

	if len(out) == 0 {
		return nil, "that file has no lessons in it"
	}
	return out[:], ""
}

item_title :: proc(it: Item) -> string {
	if it.is_range {
		if it.has_page {
			return fmt.aprintf("Lessons %d%s%d: %s (Page: %d)", it.number, EN_DASH, it.end, it.name, it.page)
		}
		return fmt.aprintf("Lessons %d%s%d: %s", it.number, EN_DASH, it.end, it.name)
	}
	if it.has_page {
		return fmt.aprintf("Lesson %d: %s (Page: %d)", it.number, it.name, it.page)
	}
	return fmt.aprintf("Lesson %d: %s", it.number, it.name)
}

item_notes :: proc(it: Item, curriculum_name: string) -> string {
	if it.unit > 0 {
		return fmt.aprintf("%s: Unit %d", curriculum_name, it.unit)
	}
	return strings.clone(curriculum_name)
}

emit_yaml :: proc(name, subject: string, items: []Item) -> string {
	b: strings.Builder
	strings.builder_init(&b)
	strings.write_string(&b, "plans:\n")
	strings.write_string(&b, "  - name: ")
	write_yaml_scalar(&b, name)
	strings.write_string(&b, "\n    subject: ")
	write_yaml_scalar(&b, subject)
	strings.write_string(&b, "\n    items:\n")
	for it in items {
		strings.write_string(&b, "      - title: ")
		write_yaml_scalar(&b, item_title(it))
		strings.write_string(&b, "\n        notes: ")
		write_yaml_scalar(&b, item_notes(it, name))
		strings.write_string(&b, "\n")
	}
	return strings.to_string(b)
}

write_yaml_scalar :: proc(b: ^strings.Builder, s: string) {
	if !yaml_needs_quote(s) {
		strings.write_string(b, s)
		return
	}
	strings.write_byte(b, '"')
	for r in s {
		if r == '"' || r == '\\' {
			strings.write_byte(b, '\\')
		}
		strings.write_rune(b, r)
	}
	strings.write_byte(b, '"')
}

yaml_needs_quote :: proc(s: string) -> bool {
	if s == "" {
		return true
	}
	for r in s {
		switch r {
		case ':', '#', '\'', '"', '{', '}', '[', ']', ',', '&', '*', '!', '|', '>', '%', '@', '`', '\\', '/', ' ', '\t':
			return true
		}
	}
	return false
}

flush_item :: proc(out: ^[dynamic]Item, cur: ^Item, open: ^bool) {
	if !open^ {
		return
	}
	cur.name = collapse_space(cur.name)
	append(out, cur^)
	open^ = false
	cur^ = {}
}

apply_fragment :: proc(cur: ^Item, fragment: string) {
	rest, page, ok := take_page(fragment)
	if ok {
		append_name(cur, rest)
		cur.page = page
		cur.has_page = true
		return
	}
	append_name(cur, fragment)
}

append_name :: proc(cur: ^Item, fragment: string) {
	bit := collapse_space(fragment)
	if bit == "" {
		return
	}
	if cur.name == "" {
		cur.name = strings.clone(bit)
		return
	}
	cur.name = fmt.aprintf("%s %s", cur.name, bit)
}

parse_unit_line :: proc(line: string) -> (n: int, ok: bool) {
	if !has_ascii_prefix_ci(line, "unit") {
		return 0, false
	}
	rest := line[4:]
	if rest == "" || !is_space(rest[0]) {
		return 0, false
	}
	rest = strings.trim_left_space(rest)
	n, rest, ok = parse_int_prefix(rest)
	if !ok || n <= 0 {
		return 0, false
	}
	return n, true
}

parse_lesson_start :: proc(line: string) -> (item: Item, rest: string, ok: bool) {
	if has_ascii_prefix_ci(line, "lessons") {
		after := line[7:]
		if after == "" || !is_space(after[0]) {
			return {}, "", false
		}
		after = strings.trim_left_space(after)
		n: int
		n, after, ok = parse_int_prefix(after)
		if !ok {
			return {}, "", false
		}
		after, ok = skip_dash(after)
		if !ok {
			return {}, "", false
		}
		after = strings.trim_left_space(after)
		m: int
		m, after, ok = parse_int_prefix(after)
		if !ok {
			return {}, "", false
		}
		after = strings.trim_left_space(after)
		if after == "" || after[0] != ':' {
			return {}, "", false
		}
		item = Item{is_range = true, number = n, end = m}
		return item, strings.trim_space(after[1:]), true
	}

	if !has_ascii_prefix_ci(line, "lesson") {
		return {}, "", false
	}
	after := line[6:]
	if after == "" || !is_space(after[0]) {
		return {}, "", false
	}
	after = strings.trim_left_space(after)
	n: int
	n, after, ok = parse_int_prefix(after)
	if !ok {
		return {}, "", false
	}
	after = strings.trim_left_space(after)
	if after == "" || after[0] != ':' {
		return {}, "", false
	}
	item = Item{number = n}
	return item, strings.trim_space(after[1:]), true
}

skip_dash :: proc(s: string) -> (rest: string, ok: bool) {
	if s == "" {
		return s, false
	}
	r, size := utf8.decode_rune_in_string(s)
	if r == '-' || r == '–' || r == '—' {
		return strings.trim_left_space(s[size:]), true
	}
	return s, false
}

take_page :: proc(src: string) -> (rest: string, page: int, ok: bool) {
	s := strings.trim_right_space(src)
	if s == "" {
		return s, 0, false
	}
	i := len(s) - 1
	for i >= 0 && s[i] >= '0' && s[i] <= '9' {
		i -= 1
	}
	digit_start := i + 1
	if digit_start >= len(s) {
		return s, 0, false
	}
	j := i
	for j >= 0 && is_space(s[j]) {
		j -= 1
	}
	dots := 0
	k := j
	for k >= 0 {
		if s[k] == '.' {
			dots += 1
			k -= 1
			for k >= 0 && is_space(s[k]) {
				k -= 1
			}
			continue
		}
		break
	}
	if dots < 2 {
		return s, 0, false
	}
	page, _ = strconv.parse_int(s[digit_start:])
	rest = strings.trim_space(s[:k + 1])
	return rest, page, true
}

parse_int_prefix :: proc(s: string) -> (n: int, rest: string, ok: bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i += 1
	}
	if i == 0 {
		return 0, s, false
	}
	n, _ = strconv.parse_int(s[:i])
	return n, s[i:], true
}

has_ascii_prefix_ci :: proc(s, prefix: string) -> bool {
	if len(s) < len(prefix) {
		return false
	}
	for i in 0 ..< len(prefix) {
		a := s[i]
		b := prefix[i]
		if a >= 'A' && a <= 'Z' {
			a += 32
		}
		if b >= 'A' && b <= 'Z' {
			b += 32
		}
		if a != b {
			return false
		}
	}
	return true
}

is_space :: proc(c: u8) -> bool {
	return c == ' ' || c == '\t'
}

collapse_space :: proc(s: string) -> string {
	b: strings.Builder
	strings.builder_init(&b)
	defer strings.builder_destroy(&b)
	prev_space := false
	for r in s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prev_space {
				strings.write_rune(&b, ' ')
			}
			prev_space = true
			continue
		}
		strings.write_rune(&b, r)
		prev_space = false
	}
	return strings.clone(strings.trim_space(strings.to_string(b)))
}
