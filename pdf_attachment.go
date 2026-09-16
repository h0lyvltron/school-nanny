package main

import "strings"

func firstPDFAttachment(files []Attachment) (Attachment, bool) {
	for _, f := range files {
		if f.ContentType == "application/pdf" {
			return f, true
		}
		if strings.HasSuffix(strings.ToLower(f.OriginalName), ".pdf") {
			return f, true
		}
	}
	return Attachment{}, false
}
