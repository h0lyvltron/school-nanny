package main

import (
	"strings"
	"testing"
)

func TestFillPageEnds(t *testing.T) {
	items := []CurriculumItem{
		{Title: "A", PageStart: 2},
		{Title: "B", PageStart: 4},
		{Title: "C", PageStart: 105},
		{Title: "D"}, // no pages
	}
	FillPageEnds(items)
	if items[0].PageEnd != 3 {
		t.Fatalf("A end: want 3 got %d", items[0].PageEnd)
	}
	if items[1].PageEnd != 104 {
		t.Fatalf("B end: want 104 got %d", items[1].PageEnd)
	}
	if items[2].PageEnd != 0 {
		t.Fatalf("C end should stay open, got %d", items[2].PageEnd)
	}
	if items[3].PageEnd != 0 {
		t.Fatalf("D should stay empty")
	}
}

func TestFillPageEndsPreservesExplicitEnd(t *testing.T) {
	items := []CurriculumItem{
		{PageStart: 10, PageEnd: 12},
		{PageStart: 20},
	}
	FillPageEnds(items)
	if items[0].PageEnd != 12 {
		t.Fatalf("explicit end overwritten: %d", items[0].PageEnd)
	}
	if items[1].PageEnd != 0 {
		t.Fatalf("last item should not invent an end")
	}
}

func TestItemsFromTOCSetsPages(t *testing.T) {
	toc := []TocItem{
		{Number: 1, Name: "Alpha", Page: 2, HasPage: true, Unit: 1},
		{Number: 2, Name: "Beta", Page: 4, HasPage: true, Unit: 1},
	}
	items := ItemsFromTOC(toc, "Demo Math")
	if len(items) != 2 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].PageStart != 2 || items[0].PageEnd != 3 {
		t.Fatalf("first pages: %+v", items[0])
	}
	if items[1].PageStart != 4 || items[1].PageEnd != 0 {
		t.Fatalf("second pages: %+v", items[1])
	}
	if !strings.Contains(items[0].Title, "Alpha") {
		t.Fatalf("title=%q", items[0].Title)
	}
}
