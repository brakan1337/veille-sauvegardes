package collecte

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

var midi = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

func recu(serveur string, il_y_a time.Duration, statuts ...rapport.Statut) Recu {
	r := rapport.Rapport{Serveur: serveur, Systeme: "linux/amd64", VersionAgent: "v1", EmisLe: midi.Add(-il_y_a)}
	for i, s := range statuts {
		r.Resultats = append(r.Resultats, rapport.Resultat{Nom: "sauvegarde " + string(rune('A'+i)), Statut: s, Detail: "détail"})
	}
	return Recu{RecuLe: midi.Add(-il_y_a), Rapport: r}
}

func TestLeDepotSurvitAUnRedemarrage(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "donnees", "rapports.jsonl")
	d, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	d.Enregistrer(recu("srv-a", 30*time.Hour, rapport.Absente))
	d.Enregistrer(recu("srv-a", 2*time.Hour, rapport.OK))
	d.Enregistrer(recu("srv-b", 3*time.Hour, rapport.OK))
	d.Enregistrer(recu("srv-a", 50*time.Hour, rapport.Erreur)) // rattrapé en retard
	d.Fermer()

	rouvert, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer rouvert.Fermer()
	derniers := rouvert.Derniers()
	if len(derniers) != 2 || derniers[0].Rapport.Serveur != "srv-a" {
		t.Fatalf("serveurs relus : %+v", derniers)
	}
	if derniers[0].Rapport.Resultats[0].Statut != rapport.OK {
		t.Error("un rapport ancien rattrapé ne doit pas remplacer le plus récent")
	}
}

func TestSynthese(t *testing.T) {
	s := Synthetiser([]Recu{
		recu("srv-compta", 2*time.Hour, rapport.OK, rapport.EnChute),
		recu("srv-fichiers", 3*time.Hour, rapport.OK),
		recu("srv-messagerie", 40*time.Hour, rapport.OK),
	}, midi, 26*time.Hour)

	if s.OK != 1 || s.Alertes != 1 || s.Muets != 1 {
		t.Fatalf("comptes faux : %d ok, %d alertes, %d muets", s.OK, s.Alertes, s.Muets)
	}
	texte := s.Texte()
	for _, attendu := range []string{
		"1 serveur(s) en ordre, 1 en alerte, 1 muet(s)",
		"srv-compta :\n  - sauvegarde B : EN_CHUTE",
		"srv-messagerie : MUET, aucun rapport depuis 1 j 16 h",
	} {
		if !strings.Contains(texte, attendu) {
			t.Errorf("le rapport du matin devrait contenir %q :\n%s", attendu, texte)
		}
	}
	if strings.Contains(texte, "srv-fichiers") {
		t.Error("un serveur en ordre n'a pas sa place dans le détail du rapport")
	}
}

func TestRienASignaler(t *testing.T) {
	texte := Synthetiser([]Recu{recu("srv", time.Hour, rapport.OK)}, midi, 26*time.Hour).Texte()
	if !strings.HasSuffix(texte, "Rien à signaler.\n") {
		t.Errorf("texte : %q", texte)
	}
}

func nouveauServeur(t *testing.T) (*Serveur, http.Handler) {
	d, err := Ouvrir(filepath.Join(t.TempDir(), "r.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Fermer() })
	s := &Serveur{
		Depot: d, JetonAgents: "jeton-des-agents", MotDePasseAdmin: "mot-de-passe-admin",
		SilenceMax: 26 * time.Hour, Horloge: func() time.Time { return midi },
		Journal: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return s, s.Routes()
}

func requete(h http.Handler, methode, url, corps string, entetes map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(methode, url, strings.NewReader(corps))
	for k, v := range entetes {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func corpsRapport(t *testing.T, serveur string, statut rapport.Statut) string {
	var b bytes.Buffer
	json.NewEncoder(&b).Encode(rapport.Rapport{
		Serveur: serveur, Systeme: "linux/amd64", VersionAgent: "v1", EmisLe: midi.Add(-time.Hour),
		Resultats: []rapport.Resultat{{Nom: "base comptable", Statut: statut, Detail: "120 Mo contre 4,1 Go"}},
	})
	return b.String()
}

func TestReceptionDesRapports(t *testing.T) {
	s, h := nouveauServeur(t)
	agent := map[string]string{"Authorization": "Bearer jeton-des-agents"}
	cas := []struct {
		nom      string
		corps    string
		entetes  map[string]string
		attendue int
	}{
		{"sans jeton", corpsRapport(t, "a", rapport.OK), nil, 401},
		{"mauvais jeton", corpsRapport(t, "a", rapport.OK), map[string]string{"Authorization": "Bearer faux"}, 401},
		{"JSON illisible", "{pas du json", agent, 400},
		{"champ inconnu", `{"serveur":"a","pirate":1}`, agent, 400},
		{"rapport invalide", `{"serveur":"","emis_le":"2026-11-16T10:00:00Z"}`, agent, 422},
		{"statut inconnu", strings.Replace(corpsRapport(t, "a", rapport.OK), `"OK"`, `"PEUT_ETRE"`, 1), agent, 422},
		{"rapport correct", corpsRapport(t, "srv-compta", rapport.EnChute), agent, 202},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			if w := requete(h, "POST", "/api/rapports", c.corps, c.entetes); w.Code != c.attendue {
				t.Errorf("code %d, attendu %d : %s", w.Code, c.attendue, w.Body.String())
			}
		})
	}
	if len(s.Depot.Derniers()) != 1 {
		t.Error("seul le rapport correct doit être enregistré")
	}
}

func TestRapportTropVolumineux(t *testing.T) {
	_, h := nouveauServeur(t)
	enorme := `{"serveur":"` + strings.Repeat("x", 2<<20) + `"}`
	w := requete(h, "POST", "/api/rapports", enorme, map[string]string{"Authorization": "Bearer jeton-des-agents"})
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("code %d, attendu 413", w.Code)
	}
}

func TestTableauDeBordProtege(t *testing.T) {
	_, h := nouveauServeur(t)
	requete(h, "POST", "/api/rapports", corpsRapport(t, "srv-compta", rapport.EnChute),
		map[string]string{"Authorization": "Bearer jeton-des-agents"})

	if w := requete(h, "GET", "/", "", nil); w.Code != 401 {
		t.Errorf("tableau sans identifiants : %d", w.Code)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("admin", "mot-de-passe-admin")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	page := w.Body.String()
	if w.Code != 200 || !strings.Contains(page, "srv-compta") || !strings.Contains(page, "120 Mo contre 4,1 Go") {
		t.Errorf("tableau de bord : %d\n%s", w.Code, page)
	}
	if w := requete(h, "GET", "/sante", "", nil); w.Code != 200 {
		t.Errorf("/sante doit rester public pour la supervision : %d", w.Code)
	}
}

func TestProchainEnvoi(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	avant := time.Date(2026, 11, 16, 6, 0, 0, 0, paris)
	apres := time.Date(2026, 11, 16, 9, 0, 0, 0, paris)
	if p, _ := ProchainEnvoi(avant, "07:30", paris); !p.Equal(time.Date(2026, 11, 16, 7, 30, 0, 0, paris)) {
		t.Errorf("avant l'heure : %v", p)
	}
	if p, _ := ProchainEnvoi(apres, "07:30", paris); !p.Equal(time.Date(2026, 11, 17, 7, 30, 0, 0, paris)) {
		t.Errorf("après l'heure : %v", p)
	}
	if _, err := ProchainEnvoi(avant, "7h30", paris); err == nil {
		t.Error("format d'heure invalide accepté")
	}
}
