// toc2yaml turns a pasted table of contents into a School Nanny curriculum YAML.
//
// Build:
//     odin build tools/toc2yaml -out:tools/toc2yaml/toc2yaml.exe
//
// Example:
//     toc2yaml -name "Grade 1 Math" -subject math -in toc.txt -out grade1-math.yaml
//
// Copy the book's table of contents into a plain text file first. Then import
// the YAML on the Curriculum page. Minutes can be filled in per lesson after.
package toc2yaml

import "core:fmt"
import "core:os"
import "core:strings"

USAGE :: `Usage: toc2yaml -name "Grade 1 Math" -subject math [-in toc.txt] [-out plan.yaml]

  -name      Curriculum name (required). Also used in each lesson's notes.
  -subject   Subject name or slug, like math or Language Arts (required).
  -in        Table of contents text file. Reads stdin if omitted.
  -out       YAML file to write. Prints to stdout if omitted.
`

main :: proc() {
	name, subject, in_path, out_path, ok := parse_flags(os.args[1:])
	if !ok {
		fmt.eprint(USAGE)
		os.exit(1)
	}

	text, read_ok := read_input(in_path)
	if !read_ok {
		if in_path == "" {
			fmt.eprintf("toc2yaml: could not read stdin\n")
		} else {
			fmt.eprintf("toc2yaml: could not read %s\n", in_path)
		}
		os.exit(1)
	}
	if strings.trim_space(text) == "" {
		fmt.eprintf("toc2yaml: that file is empty\n")
		os.exit(1)
	}

	items, err := parse_toc(text, name)
	if err != "" {
		fmt.eprintf("toc2yaml: %s\n", err)
		os.exit(1)
	}

	yaml := emit_yaml(name, subject, items)
	if out_path == "" {
		fmt.print(yaml)
		return
	}
	if os.write_entire_file(out_path, yaml) != nil {
		fmt.eprintf("toc2yaml: could not write %s\n", out_path)
		os.exit(1)
	}
}

parse_flags :: proc(args: []string) -> (name, subject, in_path, out_path: string, ok: bool) {
	for i := 0; i < len(args); i += 1 {
		a := args[i]
		value := ""
		key := a
		if eq, n, v := split_eq(a); eq {
			key = n
			value = v
		} else if i + 1 < len(args) && !strings.has_prefix(args[i + 1], "-") {
			i += 1
			value = args[i]
		} else if a == "-h" || a == "-help" || a == "--help" {
			return "", "", "", "", false
		} else {
			fmt.eprintf("toc2yaml: unknown or incomplete flag %s\n", a)
			return "", "", "", "", false
		}
		switch key {
		case "-name":
			name = value
		case "-subject":
			subject = value
		case "-in":
			in_path = value
		case "-out":
			out_path = value
		case:
			fmt.eprintf("toc2yaml: unknown flag %s\n", key)
			return "", "", "", "", false
		}
	}
	if strings.trim_space(name) == "" || strings.trim_space(subject) == "" {
		fmt.eprintf("toc2yaml: -name and -subject are required\n")
		return "", "", "", "", false
	}
	return name, subject, in_path, out_path, true
}

split_eq :: proc(a: string) -> (ok: bool, key, value: string) {
	for i in 0 ..< len(a) {
		if a[i] == '=' {
			return true, a[:i], a[i + 1:]
		}
	}
	return false, a, ""
}

read_input :: proc(path: string) -> (text: string, ok: bool) {
	data: []byte
	err: os.Error
	if path == "" {
		data, err = os.read_entire_file(os.stdin, context.allocator)
	} else {
		data, err = os.read_entire_file(path, context.allocator)
	}
	if err != nil {
		return "", false
	}
	return string(data), true
}
