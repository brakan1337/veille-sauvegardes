package agent

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/collecte"
	"github.com/brakan1337/veille-sauvegardes/internal/controle"
	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

// Bout en bout : un vrai collecteur HTTP, un agent qui contrôle de vrais fichiers et envoie son rapport.
func TestDeLAgentAuTableauDeBord(t *testing.T) {
	dossier := t.TempDir()
	maintenant := time.Now()
	sauvegarde := filepath.Join(dossier, "compta-1116.sql.gz")
	os.WriteFile(sauvegarde, make([]byte, 4096), 0o600)
	os.Chtimes(sauvegarde, maintenant.Add(-3*time.Hour), maintenant.Add(-3*time.Hour))

	depot, err := collecte.Ouvrir(filepath.Join(t.TempDir(), "rapports.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer depot.Fermer()
	serveur := &collecte.Serveur{
		Depot: depot, JetonAgents: "jeton-de-test-assez-long", MotDePasseAdmin: "x",
		SilenceMax: 26 * time.Hour, Horloge: time.Now, Journal: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	collecteur := httptest.NewServer(serveur.Routes())
	defer collecteur.Close()

	cfg := controle.Config{
		Collecteur: collecteur.URL, Jeton: "jeton-de-test-assez-long", Serveur: "srv-compta",
		Attente: filepath.Join(dossier, "attente.jsonl"),
		Sauvegardes: []controle.Sauvegarde{
			{Nom: "base comptable", Chemin: dossier, Motif: "*.sql.gz", AgeMax: controle.Duree(26 * time.Hour), TailleMin: 1024},
			{Nom: "fichiers partagés", Chemin: dossier, Motif: "*.zip", AgeMax: controle.Duree(26 * time.Hour)},
		},
	}
	r := Controler(context.Background(), cfg, "test", maintenant)
	if err := Client(cfg).Envoyer(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	s := serveur.Synthese()
	if len(s.Serveurs) != 1 || s.Serveurs[0].Serveur != "srv-compta" || s.Alertes != 1 {
		t.Fatalf("synthèse inattendue : %+v", s)
	}
	resultats := s.Serveurs[0].Resultats
	if resultats[0].Statut != rapport.OK || resultats[1].Statut != rapport.Absente {
		t.Errorf("résultats : %+v", resultats)
	}
}
