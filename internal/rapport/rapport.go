// Package rapport définit ce que l'agent envoie au collecteur : le format partagé
// par les deux programmes.
package rapport

import (
	"errors"
	"fmt"
	"time"
)

// Statut d'une sauvegarde après contrôle.
type Statut string

const (
	OK       Statut = "OK"
	Absente  Statut = "ABSENTE"  // aucun fichier ne correspond
	Ancienne Statut = "ANCIENNE" // la plus récente est plus vieille que l'âge maximal
	Petite   Statut = "PETITE"   // la plus récente est sous la taille minimale
	EnChute  Statut = "EN_CHUTE" // la plus récente a fondu par rapport à la précédente
	Erreur   Statut = "ERREUR"   // le dossier est illisible ou ne répond pas
)

// EnAlerte dit si le statut demande une intervention.
func (s Statut) EnAlerte() bool { return s != OK }

// Resultat du contrôle d'une sauvegarde.
type Resultat struct {
	Nom     string     `json:"nom"`
	Chemin  string     `json:"chemin"`
	Statut  Statut     `json:"statut"`
	Detail  string     `json:"detail"`
	Fichier string     `json:"fichier,omitempty"`
	Taille  int64      `json:"taille,omitempty"`
	Date    *time.Time `json:"date,omitempty"`
}

// Rapport envoyé par un agent après une passe de contrôle.
type Rapport struct {
	Serveur      string     `json:"serveur"`
	Systeme      string     `json:"systeme"`
	VersionAgent string     `json:"version_agent"`
	EmisLe       time.Time  `json:"emis_le"`
	Resultats    []Resultat `json:"resultats"`
}

// MaxResultats borne la taille d'un rapport : un agent mal configuré ne doit pas saturer le collecteur.
const MaxResultats = 200

// Valider refuse un rapport incomplet ou incohérent avant de l'enregistrer.
func (r Rapport) Valider() error {
	if r.Serveur == "" || len(r.Serveur) > 100 {
		return errors.New("le nom du serveur est obligatoire (100 caractères au plus)")
	}
	if r.EmisLe.IsZero() {
		return errors.New("la date d'émission est obligatoire")
	}
	if len(r.Resultats) > MaxResultats {
		return fmt.Errorf("trop de résultats : %d (maximum %d)", len(r.Resultats), MaxResultats)
	}
	for i, res := range r.Resultats {
		switch res.Statut {
		case OK, Absente, Ancienne, Petite, EnChute, Erreur:
		default:
			return fmt.Errorf("résultat %d : statut inconnu %q", i+1, res.Statut)
		}
	}
	return nil
}

// Alertes compte les sauvegardes en défaut.
func (r Rapport) Alertes() int {
	n := 0
	for _, res := range r.Resultats {
		if res.Statut.EnAlerte() {
			n++
		}
	}
	return n
}
