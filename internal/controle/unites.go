package controle

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Duree se lit « 26h », « 90m » ou « 2j » (jours) dans la configuration.
type Duree time.Duration

func (d *Duree) UnmarshalJSON(b []byte) error {
	var texte string
	if err := json.Unmarshal(b, &texte); err != nil {
		return fmt.Errorf("durée attendue entre guillemets, par exemple \"26h\" : %w", err)
	}
	v, err := LireDuree(texte)
	*d = Duree(v)
	return err
}

// LireDuree accepte les unités de Go (h, m, s) et les jours (j).
func LireDuree(texte string) (time.Duration, error) {
	texte = strings.TrimSpace(texte)
	if jours, ok := strings.CutSuffix(texte, "j"); ok {
		n, err := strconv.ParseFloat(jours, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("durée invalide %q", texte)
		}
		return time.Duration(n * 24 * float64(time.Hour)), nil
	}
	v, err := time.ParseDuration(texte)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("durée invalide %q (exemples : 26h, 90m, 2j)", texte)
	}
	return v, nil
}

// Taille se lit « 500Mo », « 2Go », « 1.5GB » ou « 0 » dans la configuration.
type Taille int64

func (t *Taille) UnmarshalJSON(b []byte) error {
	var texte string
	if err := json.Unmarshal(b, &texte); err != nil {
		return fmt.Errorf("taille attendue entre guillemets, par exemple \"500Mo\" : %w", err)
	}
	v, err := LireTaille(texte)
	*t = Taille(v)
	return err
}

var multiplicateurs = []struct {
	suffixes []string
	facteur  float64
}{
	{[]string{"To", "TB", "T"}, 1 << 40},
	{[]string{"Go", "GB", "G"}, 1 << 30},
	{[]string{"Mo", "MB", "M"}, 1 << 20},
	{[]string{"Ko", "KB", "K"}, 1 << 10},
	{[]string{"o", "B"}, 1},
}

// LireTaille convertit une taille lisible en octets (base 1024).
func LireTaille(texte string) (int64, error) {
	texte = strings.TrimSpace(texte)
	for _, m := range multiplicateurs {
		for _, s := range m.suffixes {
			if nombre, ok := strings.CutSuffix(texte, s); ok {
				n, err := strconv.ParseFloat(strings.TrimSpace(nombre), 64)
				if err != nil || n < 0 {
					return 0, fmt.Errorf("taille invalide %q", texte)
				}
				return int64(n * m.facteur), nil
			}
		}
	}
	n, err := strconv.ParseInt(texte, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("taille invalide %q (exemples : 500Mo, 2Go, 0)", texte)
	}
	return n, nil
}

// TailleLisible écrit une taille en octets comme un humain la lirait : « 1,4 Go ».
func TailleLisible(octets int64) string {
	unites := []string{"o", "Ko", "Mo", "Go", "To"}
	v := float64(octets)
	i := 0
	for v >= 1024 && i < len(unites)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d o", octets)
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", v, unites[i]), ".", ",", 1)
}

// DureeLisible écrit une durée en jours et heures : « 3 j 4 h », « 5 h ».
func DureeLisible(d time.Duration) string {
	heures := int(d.Hours())
	if heures < 1 {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	if heures < 24 {
		return fmt.Sprintf("%d h", heures)
	}
	return fmt.Sprintf("%d j %d h", heures/24, heures%24)
}
