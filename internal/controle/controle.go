// Package controle vérifie les sauvegardes présentes sur le serveur : existence,
// fraîcheur, taille, et chute brutale de taille par rapport à la précédente.
package controle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

// Sauvegarde décrit une sauvegarde à surveiller.
type Sauvegarde struct {
	Nom       string `json:"nom"`
	Chemin    string `json:"chemin"`
	Motif     string `json:"motif"`
	AgeMax    Duree  `json:"age_max"`
	TailleMin Taille `json:"taille_min"`
}

// Config est le fichier agent.json posé sur chaque serveur.
type Config struct {
	Collecteur  string       `json:"collecteur"`
	Jeton       string       `json:"jeton"`
	Serveur     string       `json:"serveur"`
	Attente     string       `json:"attente"`
	Sauvegardes []Sauvegarde `json:"sauvegardes"`
}

// Charger lit et valide la configuration de l'agent.
func Charger(chemin string) (Config, error) {
	contenu, err := os.ReadFile(chemin)
	if err != nil {
		return Config{}, fmt.Errorf("lecture de la configuration : %w", err)
	}
	var c Config
	decodeur := json.NewDecoder(strings.NewReader(string(contenu)))
	decodeur.DisallowUnknownFields()
	if err := decodeur.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("configuration %s : %w", chemin, err)
	}
	if c.Serveur == "" {
		c.Serveur, _ = os.Hostname()
	}
	return c, c.valider()
}

func (c Config) valider() error {
	var problemes []string
	if !strings.HasPrefix(c.Collecteur, "http://") && !strings.HasPrefix(c.Collecteur, "https://") {
		problemes = append(problemes, "« collecteur » doit être une adresse http(s)")
	}
	if c.Jeton == "" {
		problemes = append(problemes, "« jeton » est obligatoire")
	}
	if len(c.Sauvegardes) == 0 {
		problemes = append(problemes, "aucune sauvegarde à surveiller")
	}
	for i, s := range c.Sauvegardes {
		if s.Nom == "" || s.Chemin == "" || s.Motif == "" {
			problemes = append(problemes, fmt.Sprintf("sauvegarde %d : nom, chemin et motif sont obligatoires", i+1))
		}
		if _, err := filepath.Match(s.Motif, ""); err != nil {
			problemes = append(problemes, fmt.Sprintf("sauvegarde %d : motif invalide %q", i+1, s.Motif))
		}
		if s.AgeMax <= 0 {
			problemes = append(problemes, fmt.Sprintf("sauvegarde %d : « age_max » est obligatoire", i+1))
		}
	}
	if len(problemes) > 0 {
		return errors.New(strings.Join(problemes, " ; "))
	}
	return nil
}

// SeuilChute : la nouvelle sauvegarde est suspecte si elle pèse moins de la moitié de la précédente.
// Une base qui passe de 4 Go à 300 Mo en une nuit, c'est rarement une bonne nouvelle.
const SeuilChute = 0.5

type fichier struct {
	nom    string
	taille int64
	date   time.Time
}

// Verifier contrôle une sauvegarde à l'instant donné.
func Verifier(s Sauvegarde, maintenant time.Time) rapport.Resultat {
	res := rapport.Resultat{Nom: s.Nom, Chemin: s.Chemin}

	entrees, err := os.ReadDir(s.Chemin)
	if err != nil {
		res.Statut, res.Detail = rapport.Erreur, "dossier illisible : "+err.Error()
		return res
	}
	var fichiers []fichier
	for _, e := range entrees {
		if e.IsDir() {
			continue
		}
		if ok, _ := filepath.Match(s.Motif, e.Name()); !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // supprimé entre la lecture du dossier et celle du fichier
		}
		fichiers = append(fichiers, fichier{e.Name(), info.Size(), info.ModTime()})
	}
	if len(fichiers) == 0 {
		res.Statut, res.Detail = rapport.Absente, fmt.Sprintf("aucun fichier « %s » dans le dossier", s.Motif)
		return res
	}
	sort.Slice(fichiers, func(i, j int) bool { return fichiers[i].date.After(fichiers[j].date) })

	dernier := fichiers[0]
	res.Fichier, res.Taille = dernier.nom, dernier.taille
	date := dernier.date
	res.Date = &date

	age := maintenant.Sub(dernier.date)
	switch {
	case age > time.Duration(s.AgeMax):
		res.Statut = rapport.Ancienne
		res.Detail = fmt.Sprintf("dernière sauvegarde il y a %s (maximum %s)",
			DureeLisible(age), DureeLisible(time.Duration(s.AgeMax)))
	case dernier.taille < int64(s.TailleMin):
		res.Statut = rapport.Petite
		res.Detail = fmt.Sprintf("%s, sous le minimum de %s", TailleLisible(dernier.taille), TailleLisible(int64(s.TailleMin)))
	case len(fichiers) > 1 && float64(dernier.taille) < SeuilChute*float64(fichiers[1].taille):
		res.Statut = rapport.EnChute
		res.Detail = fmt.Sprintf("%s contre %s la fois précédente", TailleLisible(dernier.taille), TailleLisible(fichiers[1].taille))
	default:
		res.Statut = rapport.OK
		res.Detail = fmt.Sprintf("%s, il y a %s", TailleLisible(dernier.taille), DureeLisible(age))
	}
	return res
}

// VerifierTout contrôle toutes les sauvegardes en parallèle.
//
// Un partage réseau qui ne répond plus peut bloquer la lecture d'un dossier
// indéfiniment : chaque contrôle a son propre délai, et un dossier figé ne retarde
// pas les autres. Les résultats gardent l'ordre de la configuration.
func VerifierTout(ctx context.Context, sauvegardes []Sauvegarde, maintenant time.Time, delai time.Duration,
	verifier func(Sauvegarde, time.Time) rapport.Resultat) []rapport.Resultat {
	resultats := make([]rapport.Resultat, len(sauvegardes))
	var attente sync.WaitGroup
	for i, s := range sauvegardes {
		attente.Add(1)
		go func() {
			defer attente.Done()
			fini := make(chan rapport.Resultat, 1)
			go func() { fini <- verifier(s, maintenant) }()
			ctxLocal, annuler := context.WithTimeout(ctx, delai)
			defer annuler()
			select {
			case r := <-fini:
				resultats[i] = r
			case <-ctxLocal.Done():
				resultats[i] = rapport.Resultat{
					Nom: s.Nom, Chemin: s.Chemin, Statut: rapport.Erreur,
					Detail: fmt.Sprintf("le dossier ne répond pas en %s (partage réseau ?)", delai),
				}
			}
		}()
	}
	attente.Wait()
	return resultats
}
