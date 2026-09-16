package main

// FillPageEnds sets page_end from the next item's page_start when only starts
// are known (publisher TOCs usually list the first page of each lesson).
func FillPageEnds(items []CurriculumItem) {
	for i := range items {
		if items[i].PageStart <= 0 || items[i].PageEnd > 0 {
			continue
		}
		for j := i + 1; j < len(items); j++ {
			if items[j].PageStart > items[i].PageStart {
				items[i].PageEnd = items[j].PageStart - 1
				break
			}
		}
	}
}

// ItemsFromTOC turns parsed TOC rows into curriculum items with page ranges.
func ItemsFromTOC(items []TocItem, curriculumName string) []CurriculumItem {
	out := make([]CurriculumItem, 0, len(items))
	for i, it := range items {
		row := CurriculumItem{
			Title:     TOCItemTitle(it),
			Notes:     TOCItemNotes(it, curriculumName),
			SortOrder: i + 1,
		}
		if it.HasPage && it.Page > 0 {
			row.PageStart = it.Page
		}
		out = append(out, row)
	}
	FillPageEnds(out)
	return out
}
