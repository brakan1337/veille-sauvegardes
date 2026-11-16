package controle

import (
	"testing"
	"time"
)

func TestLireDuree(t *testing.T) {
	cas := map[string]time.Duration{"26h": 26 * time.Hour, "90m": 90 * time.Minute, "2j": 48 * time.Hour, "0.5j": 12 * time.Hour}
	for texte, attendu := range cas {
		if v, err := LireDuree(texte); err != nil || v != attendu {
			t.Errorf("%q : %v (%v), attendu %v", texte, v, err, attendu)
		}
	}
	for _, faux := range []string{"", "demain", "-3h", "0j"} {
		if _, err := LireDuree(faux); err == nil {
			t.Errorf("%q aurait dû être refusé", faux)
		}
	}
}

func TestLireTaille(t *testing.T) {
	cas := map[string]int64{"0": 0, "512": 512, "1Ko": 1024, "500Mo": 500 << 20, "2GB": 2 << 30, "1.5 Go": 3 << 29, "1T": 1 << 40}
	for texte, attendu := range cas {
		if v, err := LireTaille(texte); err != nil || v != attendu {
			t.Errorf("%q : %d (%v), attendu %d", texte, v, err, attendu)
		}
	}
	for _, faux := range []string{"", "beaucoup", "-1Mo", "1,5Go"} {
		if _, err := LireTaille(faux); err == nil {
			t.Errorf("%q aurait dû être refusé", faux)
		}
	}
}

func TestAffichageLisible(t *testing.T) {
	if v := TailleLisible(1503238553); v != "1,4 Go" {
		t.Errorf("TailleLisible : %q", v)
	}
	if v := TailleLisible(900); v != "900 o" {
		t.Errorf("TailleLisible : %q", v)
	}
	if v := DureeLisible(76 * time.Hour); v != "3 j 4 h" {
		t.Errorf("DureeLisible : %q", v)
	}
	if v := DureeLisible(25 * time.Minute); v != "25 min" {
		t.Errorf("DureeLisible : %q", v)
	}
}
