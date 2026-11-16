package collecte

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/controle"
	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

//go:embed tableau.html
var gabarits embed.FS

var tableau = template.Must(template.New("tableau.html").Funcs(template.FuncMap{
	"taille": controle.TailleLisible,
	"duree":  controle.DureeLisible,
	"minuscule": func(v any) string {
		switch x := v.(type) {
		case EtatServeur:
			return map[EtatServeur]string{ServeurOK: "ok", ServeurAlerte: "alerte", ServeurMuet: "muet"}[x]
		case rapport.Statut:
			if x == rapport.OK {
				return "ok"
			}
			return "alerte"
		}
		return ""
	},
}).ParseFS(gabarits, "tableau.html"))

// Serveur expose l'API des agents et le tableau de bord de l'infogérant.
type Serveur struct {
	Depot           *Depot
	JetonAgents     string // partagé par les agents de ce client
	MotDePasseAdmin string // protège le tableau de bord (utilisateur « admin »)
	SilenceMax      time.Duration
	Horloge         func() time.Time
	Journal         *slog.Logger
}

// Routes renvoie le routeur HTTP complet.
func (s *Serveur) Routes() http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("POST /api/rapports", s.recevoir)
	routes.HandleFunc("GET /api/etat", s.admin(s.etatJSON))
	routes.HandleFunc("GET /rapport.txt", s.admin(s.rapportTexte))
	routes.HandleFunc("GET /{$}", s.admin(s.tableauDeBord))
	routes.HandleFunc("GET /sante", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
	return routes
}

// Synthese calcule l'état du parc maintenant.
func (s *Serveur) Synthese() Synthese {
	return Synthetiser(s.Depot.Derniers(), s.Horloge(), s.SilenceMax)
}

func egal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Serveur) recevoir(w http.ResponseWriter, r *http.Request) {
	if !egal(r.Header.Get("Authorization"), "Bearer "+s.JetonAgents) {
		http.Error(w, "jeton absent ou invalide", http.StatusUnauthorized)
		return
	}
	var rap rapport.Rapport
	decodeur := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decodeur.DisallowUnknownFields()
	if err := decodeur.Decode(&rap); err != nil {
		var tropGros *http.MaxBytesError
		if errors.As(err, &tropGros) {
			http.Error(w, "rapport trop volumineux", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "rapport illisible : "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rap.Valider(); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := s.Depot.Enregistrer(Recu{RecuLe: s.Horloge().UTC(), Rapport: rap}); err != nil {
		s.Journal.Error("enregistrement impossible", "serveur", rap.Serveur, "erreur", err)
		http.Error(w, "enregistrement impossible", http.StatusInternalServerError)
		return
	}
	s.Journal.Info("rapport reçu", "serveur", rap.Serveur, "alertes", rap.Alertes())
	w.WriteHeader(http.StatusAccepted)
}

func (s *Serveur) admin(suite http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		utilisateur, motDePasse, ok := r.BasicAuth()
		if !ok || !egal(utilisateur, "admin") || !egal(motDePasse, s.MotDePasseAdmin) {
			w.Header().Set("WWW-Authenticate", `Basic realm="veille-sauvegardes", charset="UTF-8"`)
			http.Error(w, "identification requise", http.StatusUnauthorized)
			return
		}
		suite(w, r)
	}
}

func (s *Serveur) etatJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.Synthese())
}

func (s *Serveur) rapportTexte(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(s.Synthese().Texte()))
}

func (s *Serveur) tableauDeBord(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tableau.Execute(w, s.Synthese()); err != nil {
		s.Journal.Error("tableau de bord", "erreur", err)
	}
}
