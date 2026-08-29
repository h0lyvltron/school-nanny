package main

import (
	"fmt"
	"html/template"
	"regexp"
	"time"
)

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"prettyDate": prettyDate,
		"dayName":    func(d string) string { return formatDate(d, "Monday") },
		"dayShort":   func(d string) string { return formatDate(d, "Mon") },
		"monthDay":   func(d string) string { return formatDate(d, "Jan 2") },
		"isToday":    func(d string) bool { return d == today() },
		"isPast":     func(d string) bool { return d < today() },
		"addDays":    addDays,
		"dict":       dict,
		"initial":    initial,
		"color":      safeColor,
	}
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// safeColor lets a stored colour into a style attribute only when it really is
// a hex colour, which keeps template escaping from mangling it.
func safeColor(value string) template.CSS {
	if hexColor.MatchString(value) {
		return template.CSS(value)
	}
	return template.CSS("#5b8def")
}

func formatDate(value, layout string) string {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return value
	}
	return t.Format(layout)
}

// prettyDate keeps the calendar readable by naming the days people think in.
func prettyDate(value string) string {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return value
	}
	switch value {
	case today():
		return "Today"
	case addDays(today(), -1):
		return "Yesterday"
	case addDays(today(), 1):
		return "Tomorrow"
	}
	return t.Format("Mon, Jan 2")
}

func addDays(value string, days int) string {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return value
	}
	return t.AddDate(0, 0, days).Format(dateLayout)
}

// dict builds a map inline so partials can take more than one argument.
func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict needs an even number of arguments")
	}
	out := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		out[key] = values[i+1]
	}
	return out, nil
}

func initial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}
