package toc2yaml

import "core:testing"

@(test)
test_skips_front_matter_and_parses_units :: proc(t: ^testing.T) {
	src := string(#load("testdata/math1-toc.txt"))
	items, err := parse_toc(src, "Grade 1 Math")
	testing.expect_value(t, err, "")
	testing.expect_value(t, len(items), 6)

	testing.expect_value(t, item_title(items[0]), "Lesson 1: Identifying Right and Left (Page: 2)")
	testing.expect_value(t, item_notes(items[0], "Grade 1 Math"), "Grade 1 Math: Unit 1")
	testing.expect_value(t, items[0].page, 2)

	testing.expect_value(t, item_title(items[1]), "Lesson 2: Writing Numbers 1–5 (Page: 4)")
	testing.expect_value(t, items[1].unit, 1)
}

@(test)
test_wrapped_lessons :: proc(t: ^testing.T) {
	src := string(#load("testdata/math1-toc.txt"))
	items, err := parse_toc(src, "Grade 1 Math")
	testing.expect_value(t, err, "")

	testing.expect_value(t, item_title(items[2]), "Lesson 11: Writing One, Two, Three/ Order of Events (Page: 33)")
	testing.expect_value(t, items[2].page, 33)

	testing.expect_value(t, item_title(items[3]), "Lesson 34: Using a Number Line for Numbers Up to 40 (Page: 92)")
	testing.expect_value(t, items[3].page, 92)
}

@(test)
test_assessment_range_and_next_unit :: proc(t: ^testing.T) {
	src := string(#load("testdata/math1-toc.txt"))
	items, err := parse_toc(src, "Grade 1 Math")
	testing.expect_value(t, err, "")

	testing.expect_value(t, item_title(items[4]), "Lessons 39–40: Unit Assessment (Page: 105)")
	testing.expect_value(t, item_notes(items[4], "Grade 1 Math"), "Grade 1 Math: Unit 1")
	testing.expect(t, items[4].is_range)
	testing.expect_value(t, items[4].number, 39)
	testing.expect_value(t, items[4].end, 40)
	testing.expect_value(t, items[4].page, 105)

	testing.expect_value(t, item_title(items[5]), "Lesson 41: Writing Four, Five, Six (Page: 112)")
	testing.expect_value(t, item_notes(items[5], "Grade 1 Math"), "Grade 1 Math: Unit 2")
}

@(test)
test_hyphen_range :: proc(t: ^testing.T) {
	src := "Unit 1\nLessons 39-40: Unit Assessment . . . 105\n"
	items, err := parse_toc(src, "Grade 1 Math")
	testing.expect_value(t, err, "")
	testing.expect_value(t, len(items), 1)
	testing.expect_value(t, item_title(items[0]), "Lessons 39–40: Unit Assessment (Page: 105)")
}

@(test)
test_empty_is_an_error :: proc(t: ^testing.T) {
	_, err := parse_toc("Table of Contents\n\n", "Grade 1 Math")
	testing.expect_value(t, err, "that file has no lessons in it")
}

@(test)
test_yaml_quotes_titles :: proc(t: ^testing.T) {
	src := string(#load("testdata/math1-toc.txt"))
	items, err := parse_toc(src, "Grade 1 Math")
	testing.expect_value(t, err, "")
	yaml := emit_yaml("Grade 1 Math", "math", items)
	testing.expect(t, strings_contains(yaml, "subject: math"))
	testing.expect(t, strings_contains(yaml, `title: "Lesson 1: Identifying Right and Left (Page: 2)"`))
	testing.expect(t, strings_contains(yaml, `notes: "Grade 1 Math: Unit 1"`))
	testing.expect(t, !strings_contains(yaml, "minutes:"))
}

strings_contains :: proc(s, sub: string) -> bool {
	if len(sub) == 0 {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i in 0 ..= len(s) - len(sub) {
		if s[i:i + len(sub)] == sub {
			return true
		}
	}
	return false
}
