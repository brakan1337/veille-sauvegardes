package controle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

var maintenant = time.Date(2026, 11, 16, 7, 0, 0, 0, time.UTC)

// poser crée un fichier de la taille voulue, daté d'il y a « age ».
func poser(t *testing.T, dossier, nom string, taille int, age time.Duration) {
	t.Helper()
	chemin := filepath.Join(dossier, nom)
	if err := os.WriteFile(chemin, make([]byte, taille), 0o600); err != nil {
		t.Fatal(err)
	}
	date := maintenant.Add(-age)
	if err := os.Chtimes(chemin, date, date); err != nil {
		t.Fatal(err)
	}
}

func sauvegarde(dossier string) Sauvegarde {
	return Sauvegarde{Nom: "base", Chemin: dossier, Motif: "*.sql.gz", AgeMax: Duree(26 * time.Hour), TailleMin: 1000}
}

func TestVerifier(t *testing.T) {
	cas := []struct {
		nom     string
		poser   func(t *testing.T, d string)
		attendu rapport.Statut
		detail  string
	}{
		{"sauvegarde de cette nuit", func(t *testing.T, d string) {
			poser(t, d, "b-1115.sql.gz", 5000, 28*time.Hour)
			poser(t, d, "b-1116.sql.gz", 5200, 4*time.Hour)
		}, rapport.OK, "il y a 4 h"},
		{"aucun fichier", func(t *testing.T, d string) {}, rapport.Absente, "aucun fichier"},
		{"seuls d'autres fichiers", func(t *testing.T, d string) {
			poser(t, d, "notes.txt", 5000, time.Hour)
		}, rapport.Absente, "*.sql.gz"},
		{"trop ancienne", func(t *testing.T, d string) {
			poser(t, d, "b.sql.gz", 5000, 76*time.Hour)
		}, rapport.Ancienne, "il y a 3 j 4 h"},
		{"trop petite", func(t *testing.T, d string) {
			poser(t, d, "b.sql.gz", 200, time.Hour)
		}, rapport.Petite, "sous le minimum"},
		{"taille en chute", func(t *testing.T, d string) {
			poser(t, d, "b-1115.sql.gz", 10000, 28*time.Hour)
			poser(t, d, "b-1116.sql.gz", 3000, 4*time.Hour)
		}, rapport.EnChute, "la fois précédente"},
		{"baisse normale", func(t *testing.T, d string) {
			poser(t, d, "b-1115.sql.gz", 10000, 28*time.Hour)
			poser(t, d, "b-1116.sql.gz", 6000, 4*time.Hour)
		}, rapport.OK, ""},
		{"l'ancienneté passe avant la taille", func(t *testing.T, d string) {
			poser(t, d, "b.sql.gz", 10, 80*time.Hour)
		}, rapport.Ancienne, ""},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			dossier := t.TempDir()
			c.poser(t, dossier)
			res := Verifier(sauvegarde(dossier), maintenant)
			if res.Statut != c.attendu {
				t.Fatalf("statut %s, attendu %s (%s)", res.Statut, c.attendu, res.Detail)
			}
			if !strings.Contains(res.Detail, c.detail) {
				t.Errorf("détail %q ne contient pas %q", res.Detail, c.detail)
			}
		})
	}
}

func TestVerifierRetientLePlusRecent(t *testing.T) {
	dossier := t.TempDir()
	poser(t, dossier, "b-1116.sql.gz", 5000, 2*time.Hour)
	poser(t, dossier, "b-1114.sql.gz", 5000, 50*time.Hour)
	res := Verifier(sauvegarde(dossier), maintenant)
	if res.Fichier != "b-1116.sql.gz" || res.Taille != 5000 || res.Date == nil {
		t.Fatalf("mauvais fichier retenu : %+v", res)
	}
}

func TestDossierIllisible(t *testing.T) {
	res := Verifier(sauvegarde(filepath.Join(t.TempDir(), "absent")), maintenant)
	if res.Statut != rapport.Erreur {
		t.Fatalf("statut %s, attendu ERREUR", res.Statut)
	}
}

func TestUnDossierFigeNeRetardePasLesAutres(t *testing.T) {
	bloque := make(chan struct{})
	defer close(bloque)
	verifier := func(s Sauvegarde, _ time.Time) rapport.Resultat {
		if s.Nom == "partage figé" {
			<-bloque
		}
		return rapport.Resultat{Nom: s.Nom, Statut: rapport.OK}
	}
	sauvegardes := []Sauvegarde{{Nom: "partage figé"}, {Nom: "disque local"}}

	debut := time.Now()
	res := VerifierTout(context.Background(), sauvegardes, maintenant, 50*time.Millisecond, verifier)
	if time.Since(debut) > 2*time.Second {
		t.Fatal("le dossier figé a bloqué la passe")
	}
	if res[0].Statut != rapport.Erreur || !strings.Contains(res[0].Detail, "ne répond pas") {
		t.Errorf("le dossier figé devrait être en erreur : %+v", res[0])
	}
	if res[1].Nom != "disque local" || res[1].Statut != rapport.OK {
		t.Errorf("l'ordre ou le résultat du second contrôle est faux : %+v", res[1])
	}
}

func TestChargerConfiguration(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "agent.json")
	os.WriteFile(chemin, []byte(`{
		"collecteur": "https://collecte.exemple.test",
		"jeton": "abc",
		"serveur": "srv-compta",
		"sauvegardes": [{"nom": "base", "chemin": "/sauvegardes", "motif": "*.gz", "age_max": "2j", "taille_min": "1,5Go"}]
	}`), 0o600)
	if _, err := Charger(chemin); err == nil || !strings.Contains(err.Error(), "taille invalide") {
		t.Fatalf("une taille avec virgule devrait être refusée clairement, erreur : %v", err)
	}

	os.WriteFile(chemin, []byte(`{
		"collecteur": "https://collecte.exemple.test",
		"jeton": "abc",
		"serveur": "srv-compta",
		"sauvegardes": [{"nom": "base", "chemin": "/sauvegardes", "motif": "*.gz", "age_max": "2j", "taille_min": "1.5Go"}]
	}`), 0o600)
	cfg, err := Charger(chemin)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Sauvegardes[0]
	if time.Duration(s.AgeMax) != 48*time.Hour || int64(s.TailleMin) != 1610612736 {
		t.Errorf("unités mal lues : %v, %d", time.Duration(s.AgeMax), s.TailleMin)
	}
}

func TestConfigurationIncomplete(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "agent.json")
	os.WriteFile(chemin, []byte(`{"collecteur": "ftp://x", "sauvegardes": [{"nom": "a"}]}`), 0o600)
	_, err := Charger(chemin)
	for _, attendu := range []string{"http(s)", "jeton", "nom, chemin et motif", "age_max"} {
		if err == nil || !strings.Contains(err.Error(), attendu) {
			t.Errorf("l'erreur devrait mentionner %q : %v", attendu, err)
		}
	}
}

func TestChampInconnuRefuse(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "agent.json")
	os.WriteFile(chemin, []byte(`{"collecteur": "https://x", "jeton": "a", "agemax": "1h", "sauvegardes": []}`), 0o600)
	if _, err := Charger(chemin); err == nil || !strings.Contains(err.Error(), "agemax") {
		t.Fatalf("une faute de frappe dans un champ doit être signalée : %v", err)
	}
}
