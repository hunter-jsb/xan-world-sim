package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestHelpTree_Shape: the root reads in order of need — keys, legend,
// then the systems and lore folders — and every page in the tree has
// a unique title, content, and lines that fit the popup.
func TestHelpTree_Shape(t *testing.T) {
	m := &model{legend: "line one\nline two"}
	root := m.helpTree()
	if len(root) != 4 {
		t.Fatalf("root has %d entries, want 4 (keys, legend, systems, lore)", len(root))
	}
	if root[0].title != "keys" {
		t.Errorf("first root entry %q, want keys at the top", root[0].title)
	}
	if !strings.Contains(root[1].title, "legend") {
		t.Errorf("second root entry %q, want the legend", root[1].title)
	}
	if root[2].title != "systems" || len(root[2].children) < 4 {
		t.Errorf("third entry should be the systems folder with its topics, got %q (%d children)",
			root[2].title, len(root[2].children))
	}
	if root[3].title != "lore" || len(root[3].children) < 3 {
		t.Errorf("fourth entry should be the lore folder, got %q (%d children)",
			root[3].title, len(root[3].children))
	}

	seen := map[string]bool{}
	var walk func(nodes []helpNode)
	walk = func(nodes []helpNode) {
		for _, n := range nodes {
			if n.title == "" {
				t.Error("node without a title")
			}
			if seen[n.title] {
				t.Errorf("duplicate title %q", n.title)
			}
			seen[n.title] = true
			if len(n.children) == 0 && len(n.body) == 0 {
				t.Errorf("page %q is empty", n.title)
			}
			for _, l := range n.body {
				if utf8.RuneCountInString(l) > 96 {
					t.Errorf("page %q line overflows the popup (%d runes)", n.title, utf8.RuneCountInString(l))
				}
			}
			walk(n.children)
		}
	}
	walk(root)
}

// TestHelpTree_Navigation: folders open as submenus with a Back,
// pages link back to their menu, and ascending selects the folder you
// came from.
func TestHelpTree_Navigation(t *testing.T) {
	m := &model{legend: "legend"}

	m.openHelpPopup()
	if got, want := len(m.popup.opts), 5; got != want {
		t.Fatalf("root menu has %d options, want %d (4 entries + Close)", got, want)
	}
	if !strings.Contains(m.popup.opts[2].label, "▸") {
		t.Errorf("systems entry %q should carry a folder marker", m.popup.opts[2].label)
	}

	// Descend into lore (index 3): submenu with its pages + Back.
	m.openHelpEntry(3)
	if len(m.helpPath) != 1 || m.helpPath[0] != 3 {
		t.Fatalf("helpPath = %v, want [3]", m.helpPath)
	}
	if m.popup.title != "lore" {
		t.Errorf("submenu title %q, want lore", m.popup.title)
	}
	last := m.popup.opts[len(m.popup.opts)-1]
	if last.action != popHelpUp {
		t.Errorf("submenu's last option action %q, want Back (help-up)", last.action)
	}

	// Open a lore page; its Back returns to the lore menu on the page.
	m.openHelpEntry(1)
	if m.popup.opts[0].action != popHelpMenu {
		t.Errorf("page's first option action %q, want back-to-menu", m.popup.opts[0].action)
	}
	m.openHelpMenu(m.helpEntrySel)
	if m.popup.title != "lore" || m.popup.sel != 1 {
		t.Errorf("back landed on %q sel %d, want lore sel 1", m.popup.title, m.popup.sel)
	}

	// Up from lore returns to the root with lore selected.
	m.helpUp()
	if len(m.helpPath) != 0 || m.popup.sel != 3 {
		t.Errorf("up landed at path %v sel %d, want root with lore (3) selected", m.helpPath, m.popup.sel)
	}
}
