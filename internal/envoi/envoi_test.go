package envoi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

func unRapport(serveur string) rapport.Rapport {
	return rapport.Rapport{Serveur: serveur, EmisLe: time.Date(2026, 11, 16, 6, 30, 0, 0, time.UTC)}
}

// collecteurFactice répond avec les codes donnés, un par appel, puis 202.
func collecteurFactice(t *testing.T, codes ...int) (*httptest.Server, *atomic.Int32) {
	var appels atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(appels.Add(1))
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("jeton non transmis : %q", r.Header.Get("Authorization"))
		}
		if n <= len(codes) {
			w.WriteHeader(codes[n-1])
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	return srv, &appels
}

func client(url, dossier string) Client {
	return Client{URL: url, Jeton: "secret", Tentatives: 3, Pause: time.Millisecond, Attente: filepath.Join(dossier, "attente.jsonl")}
}

func TestReessaieApresUnePanneDuCollecteur(t *testing.T) {
	srv, appels := collecteurFactice(t, 503, 500)
	if err := client(srv.URL, t.TempDir()).Envoyer(context.Background(), unRapport("a")); err != nil {
		t.Fatal(err)
	}
	if appels.Load() != 3 {
		t.Errorf("%d appels, attendu 3", appels.Load())
	}
}

func TestUnRefusNEstPasRetente(t *testing.T) {
	srv, appels := collecteurFactice(t, 401)
	c := client(srv.URL, t.TempDir())
	err := c.Envoyer(context.Background(), unRapport("a"))
	var definitive ErreurDefinitive
	if !errors.As(err, &definitive) || definitive.Code != 401 {
		t.Fatalf("erreur %v, attendu un refus définitif 401", err)
	}
	if appels.Load() != 1 {
		t.Errorf("%d appels : un jeton refusé ne doit pas être renvoyé", appels.Load())
	}
	if _, err := os.Stat(c.Attente); !errors.Is(err, os.ErrNotExist) {
		t.Error("un rapport refusé ne doit pas être mis en attente")
	}
}

func TestCollecteurInjoignablePuisRattrapage(t *testing.T) {
	dossier := t.TempDir()
	hors := client("http://127.0.0.1:1", dossier) // aucun service n'écoute sur ce port
	hors.Tentatives = 1
	for _, serveur := range []string{"lundi", "mardi"} {
		if err := hors.Envoyer(context.Background(), unRapport(serveur)); err == nil {
			t.Fatal("l'envoi aurait dû échouer")
		}
	}

	srv, appels := collecteurFactice(t)
	retour := client(srv.URL, dossier)
	n, err := retour.Rattraper(context.Background())
	if err != nil || n != 2 || appels.Load() != 2 {
		t.Fatalf("rattrapage : %d envoyés, %d appels, erreur %v", n, appels.Load(), err)
	}
	if _, err := os.Stat(retour.Attente); !errors.Is(err, os.ErrNotExist) {
		t.Error("le fichier d'attente doit disparaître une fois vidé")
	}
}

func TestLaFileDAttenteEstBornee(t *testing.T) {
	c := client("http://127.0.0.1:1", t.TempDir())
	c.Tentatives = 1
	for i := 0; i < MaxEnAttente+5; i++ {
		c.Envoyer(context.Background(), unRapport("srv"))
	}
	rapports, err := c.lireAttente()
	if err != nil || len(rapports) != MaxEnAttente {
		t.Fatalf("%d rapports en attente (%v), attendu %d", len(rapports), err, MaxEnAttente)
	}
}
