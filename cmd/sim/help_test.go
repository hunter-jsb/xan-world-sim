package main

import (
	"testing"
	"unicode/utf8"
)

// TestHelpTopics: every page has a unique title, non-empty content,
// and lines that fit the popup's content width without truncation.
func TestHelpTopics(t *testing.T) {
	m := &model{legend: "line one\nline two"}
	topics := m.helpTopics()
	if len(topics) < 5 {
		t.Fatalf("only %d help topics — the browser lost pages", len(topics))
	}
	seen := map[string]bool{}
	for _, topic := range topics {
		if topic.title == "" || len(topic.body) == 0 {
			t.Errorf("topic %q is empty", topic.title)
		}
		if seen[topic.title] {
			t.Errorf("duplicate topic title %q", topic.title)
		}
		seen[topic.title] = true
		for _, l := range topic.body {
			if utf8.RuneCountInString(l) > 96 {
				t.Errorf("topic %q line overflows the popup (%d runes): %q",
					topic.title, utf8.RuneCountInString(l), l)
			}
		}
	}
}

// TestHelpBrowserNavigation: the menu lists every topic plus Close,
// pages carry a way back, and the menu remembers the last page read.
func TestHelpBrowserNavigation(t *testing.T) {
	m := &model{legend: "legend"}
	m.openHelpPopup()
	topics := m.helpTopics()
	if got, want := len(m.popup.opts), len(topics)+1; got != want {
		t.Fatalf("menu has %d options, want %d (topics + Close)", got, want)
	}
	if m.popup.opts[0].action != popHelpTopic {
		t.Errorf("first menu option action %q, want %q", m.popup.opts[0].action, popHelpTopic)
	}
	if m.popup.opts[len(m.popup.opts)-1].action != popClose {
		t.Error("last menu option should be Close")
	}

	m.openHelpTopic(3)
	if m.popup.title != topics[3].title {
		t.Errorf("topic page title %q, want %q", m.popup.title, topics[3].title)
	}
	if m.popup.opts[0].action != popHelpMenu {
		t.Error("topic page's first option should lead back to the menu")
	}

	m.openHelpPopup()
	if m.popup.sel != 3 {
		t.Errorf("menu reopened at %d, want 3 (the last page read)", m.popup.sel)
	}
}
