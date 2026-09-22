package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
)

// EmitAdultEventsICS writes a calendar of events. Yearly masters keep their
// RRULE so a phone that imports the file can expand them itself. EndsOn is
// inclusive in School Nanny; ICS DTEND for all-day events is exclusive.
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
		b.WriteString(string(EmitAdultEventVEVENT(e)))
	}
	b.WriteString("END:VCALENDAR\r\n")
	return []byte(b.String())
}

// EmitAdultEventVEVENT writes one VEVENT block (without the outer VCALENDAR).
func EmitAdultEventVEVENT(e AdultEvent) []byte {
	var b strings.Builder
	uid := strings.TrimSpace(e.UID)
	if uid == "" {
		if e.ID != 0 {
			uid = fmt.Sprintf("adult-event-%d@school-nanny", e.ID)
		} else {
			uid = fmt.Sprintf("adult-event-%s-%s@school-nanny", e.StartsOn, icsEscape(e.Title))
		}
	}
	stamp := e.ModifiedAt
	if stamp == "" {
		stamp = time.Now().UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse(time.RFC3339, stamp); err == nil {
		stamp = t.UTC().Format("20060102T150405Z")
	} else {
		stamp = time.Now().UTC().Format("20060102T150405Z")
	}

	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString("UID:" + uid + "\r\n")
	b.WriteString("DTSTAMP:" + stamp + "\r\n")
	b.WriteString(fmt.Sprintf("SEQUENCE:%d\r\n", e.Sequence))
	if e.AllDay || e.StartAt == "" {
		endExclusive := addDays(e.EndsOn, 1)
		b.WriteString("DTSTART;VALUE=DATE:" + icsDate(e.StartsOn) + "\r\n")
		b.WriteString("DTEND;VALUE=DATE:" + icsDate(endExclusive) + "\r\n")
	} else {
		b.WriteString("DTSTART:" + icsDateTimeUTC(e.StartAt) + "\r\n")
		end := e.EndAt
		if end == "" {
			end = e.StartAt
		}
		b.WriteString("DTEND:" + icsDateTimeUTC(end) + "\r\n")
	}
	b.WriteString("SUMMARY:" + icsEscape(e.Title) + "\r\n")
	if strings.TrimSpace(e.Body) != "" {
		b.WriteString("DESCRIPTION:" + icsEscape(e.Body) + "\r\n")
	}
	if strings.TrimSpace(e.Location) != "" {
		b.WriteString("LOCATION:" + icsEscape(e.Location) + "\r\n")
	}
	if strings.TrimSpace(e.RRule) != "" {
		b.WriteString("RRULE:" + strings.TrimSpace(e.RRule) + "\r\n")
	}
	for _, ex := range strings.Split(e.ExDates, ",") {
		ex = strings.TrimSpace(ex)
		if ex == "" {
			continue
		}
		b.WriteString("EXDATE;VALUE=DATE:" + icsDate(ex) + "\r\n")
	}
	b.WriteString("END:VEVENT\r\n")
	return []byte(b.String())
}

func icsDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return strings.ReplaceAll(iso, "-", "")
	}
	return t.Format("20060102")
}

func icsDateTimeUTC(raw string) string {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC().Format("20060102T150405Z")
	}
	return strings.TrimSpace(raw)
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

func unfoldICS(body []byte) []byte {
	// RFC 5545: lines starting with space/tab continue the previous line.
	var out bytes.Buffer
	r := bufio.NewReader(bytes.NewReader(body))
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if (line[0] == ' ' || line[0] == '\t') && out.Len() > 0 {
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
