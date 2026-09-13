package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
)

// EmitAdultEventsICS writes a calendar of all-day events. EndsOn is inclusive
// in School Nanny; ICS DTEND is exclusive, so we bump it by one day.
func EmitAdultEventsICS(calName string, events []AdultEvent) []byte {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//School Nanny//EN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	if calName != "" {
		b.WriteString("X-WR-CALNAME:" + icsEscape(calName) + "\r\n")
	}
	for _, e := range events {
		uid := fmt.Sprintf("adult-event-%d@school-nanny", e.ID)
		if e.ID == 0 {
			uid = fmt.Sprintf("adult-event-%s-%s@school-nanny", e.StartsOn, icsEscape(e.Title))
		}
		endExclusive := addDays(e.EndsOn, 1)
		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString("UID:" + uid + "\r\n")
		b.WriteString("DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("DTSTART;VALUE=DATE:" + icsDate(e.StartsOn) + "\r\n")
		b.WriteString("DTEND;VALUE=DATE:" + icsDate(endExclusive) + "\r\n")
		b.WriteString("SUMMARY:" + icsEscape(e.Title) + "\r\n")
		if strings.TrimSpace(e.Body) != "" {
			b.WriteString("DESCRIPTION:" + icsEscape(e.Body) + "\r\n")
		}
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return []byte(b.String())
}

func icsDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return strings.ReplaceAll(iso, "-", "")
	}
	return t.Format("20060102")
}

func icsEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `;`, `\;`)
	s = strings.ReplaceAll(s, `,`, `\,`)
	s = strings.ReplaceAll(s, "\r\n", `\n`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func icsUnescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n', 'N':
				b.WriteByte('\n')
			case ',', ';', '\\':
				b.WriteByte(s[i+1])
			default:
				b.WriteByte(s[i+1])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ParseICSEvents reads all-day (or date-truncated) VEVENTs into AdultEvents.
// AdultID is left zero for the caller to fill. Timed events become the local date.
func ParseICSEvents(body []byte) ([]AdultEvent, error) {
	body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	body = unfoldICS(body)
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var out []AdultEvent
	var cur *AdultEvent
	var start, end string
	flush := func() {
		if cur == nil || strings.TrimSpace(cur.Title) == "" || start == "" {
			cur = nil
			return
		}
		if end == "" {
			end = start
		} else {
			// ICS DTEND is exclusive for all-day events.
			prev := addDays(end, -1)
			if prev >= start {
				end = prev
			} else {
				end = start
			}
		}
		cur.StartsOn = start
		cur.EndsOn = end
		out = append(out, *cur)
		cur = nil
		start, end = "", ""
	}

	for sc.Scan() {
		line := sc.Text()
		upper := strings.ToUpper(line)
		switch {
		case upper == "BEGIN:VEVENT":
			flush()
			cur = &AdultEvent{}
			start, end = "", ""
		case upper == "END:VEVENT":
			flush()
		case cur == nil:
			continue
		case strings.HasPrefix(upper, "SUMMARY"):
			cur.Title = icsUnescape(icsValue(line))
		case strings.HasPrefix(upper, "DESCRIPTION"):
			cur.Body = icsUnescape(icsValue(line))
		case strings.HasPrefix(upper, "DTSTART"):
			start = parseICSDateValue(icsValue(line))
		case strings.HasPrefix(upper, "DTEND"):
			end = parseICSDateValue(icsValue(line))
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("that calendar has no events we can read")
	}
	return out, nil
}

func unfoldICS(body []byte) []byte {
	// RFC 5545: lines starting with space/tab continue the previous line.
	var out bytes.Buffer
	r := bufio.NewReader(bytes.NewReader(body))
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if (line[0] == ' ' || line[0] == '\t') && out.Len() > 0 {
				// drop prior newline and the leading whitespace
				buf := out.Bytes()
				if len(buf) > 0 && buf[len(buf)-1] == '\n' {
					out.Truncate(out.Len() - 1)
				}
				out.Write(bytes.TrimLeft(line, " \t"))
			} else {
				out.Write(line)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
	return out.Bytes()
}

func icsValue(line string) string {
	if i := strings.IndexByte(line, ':'); i >= 0 {
		return line[i+1:]
	}
	return ""
}

func parseICSDateValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 8 {
		d := raw[:8]
		if t, err := time.Parse("20060102", d); err == nil {
			return t.Format("2006-01-02")
		}
	}
	if t, err := time.Parse("20060102T150405Z", raw); err == nil {
		return t.Format("2006-01-02")
	}
	if t, err := time.Parse("20060102T150405", raw); err == nil {
		return t.Format("2006-01-02")
	}
	return ""
}
