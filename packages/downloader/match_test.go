package downloader

import "testing"

func TestArtistMatches(t *testing.T) {
	cases := []struct {
		want, got string
		match     bool
		why       string
	}{
		{"Ado", "Ado", true, "identical"},
		{"Ado", "ado", true, "case"},
		{"Ado", "Ado & 初音ミク", true, "collaboration credit"},
		{"Ado & 初音ミク", "Ado", true, "either side may carry the extra names"},
		{"Ado", "adonä", false, "prefix is not the same artist — the reason this exists"},
		{"Ado", "You'll Never Get To Heaven", false, "another artist entirely"},
		{"Ado", "Noir", false, "another artist entirely"},
		{"Ado", "Adorn", false, "substring is not a match"},
		{"Ado", "Shadow", false, "no relation"},
		{"Björk", "Bjork", true, "accents folded"},
		{"Simon & Garfunkel", "Simon and Garfunkel", true, "spelling of the conjunction"},
		{"Ado (Japanese singer)", "Ado", true, "disambiguation stripped"},
		{"Ado", "Ado feat. X", true, "featured credit is not the album artist"},
		{"Ado", "", false, "empty credit"},
		{"", "Ado", false, "no reference"},
		{"ＫＡＮＯ", "KANO", false, "full-width scripts are left alone"},
	}

	for _, c := range cases {
		if got := ArtistMatches(c.want, c.got); got != c.match {
			t.Errorf("ArtistMatches(%q, %q) = %v, want %v (%s)", c.want, c.got, got, c.match, c.why)
		}
	}
}

func TestTitleRank(t *testing.T) {
	cases := []struct {
		want, got string
		min, max  int
	}{
		{"DND", "DND", 100, 100},
		{"DND", "dnd", 100, 100},
		{"DND", "DND (Deluxe Edition)", 80, 80},
		{"DND", "DND - Remastered", 80, 80},
		{"LEMONADE", "LEMONADE - The 2nd Album", 60, 60},
		{"Raven Sky", "Something Else", 0, 0},
	}

	for _, c := range cases {
		got := TitleRank(c.want, c.got)
		if got < c.min || got > c.max {
			t.Errorf("TitleRank(%q, %q) = %d, want %d..%d", c.want, c.got, got, c.min, c.max)
		}
	}
}

func TestNoisyRelease(t *testing.T) {
	cases := []struct {
		want, got string
		noisy     bool
	}{
		{"DND", "DND", false},
		{"DND", "DND (Zedd Remix)", true},
		{"DND", "DND - Instrumental", true},
		{"DND", "DND (Sped Up)", true},
		{"DND", "DND (Deluxe Edition)", false},
		{"DND (Remix)", "DND (Remix)", false},
	}

	for _, c := range cases {
		if got := NoisyRelease(c.want, c.got); got != c.noisy {
			t.Errorf("NoisyRelease(%q, %q) = %v, want %v", c.want, c.got, got, c.noisy)
		}
	}
}
