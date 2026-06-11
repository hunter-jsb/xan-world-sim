package world

import "testing"

func TestEraForKya_Bands(t *testing.T) {
	cases := []struct {
		kya  int
		want Era
	}{
		{0, EraNow},
		{15, EraNow},
		{16, Era("16kya")},
		{194, Era("194kya")},
		{195, EraOldWorld},
		{205, EraOldWorld},
		{215, EraOldWorld},
		{216, Era("216kya")},
		{KyaMax, Era("300kya")},
	}
	for _, c := range cases {
		if got := EraForKya(c.kya); got != c.want {
			t.Errorf("EraForKya(%d) = %q, want %q", c.kya, got, c.want)
		}
	}
}

func TestParseEra(t *testing.T) {
	for _, valid := range []string{"now", "205kya"} {
		e, err := ParseEra(valid)
		if err != nil || string(e) != valid {
			t.Errorf("ParseEra(%q) = %q, %v; want %q, nil", valid, e, err, valid)
		}
	}
	if _, err := ParseEra("jurassic"); err == nil {
		t.Error("ParseEra(invalid) returned nil error")
	}
}

func TestEra_KyaAndOther(t *testing.T) {
	if EraNow.Kya() != KyaNow || EraOldWorld.Kya() != KyaOldWorld {
		t.Errorf("Era.Kya: now=%d oldworld=%d, want %d, %d",
			EraNow.Kya(), EraOldWorld.Kya(), KyaNow, KyaOldWorld)
	}
	if EraNow.Other() != EraOldWorld || EraOldWorld.Other() != EraNow {
		t.Error("Era.Other does not toggle between the named eras")
	}
}
