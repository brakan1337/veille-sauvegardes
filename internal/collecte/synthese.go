package collecte

import (
	"fmt"
	"strings"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/controle"
	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

// EtatServeur résume un serveur pour le tableau de bord.
type EtatServeur string

const (
	ServeurOK     EtatServeur = "OK"
	ServeurAlerte EtatServeur = "ALERTE"
	// ServeurMuet : aucun rapport depuis trop longtemps. Un agent arrêté ne signale
	// aucune panne : c'est aussi grave qu'une sauvegarde ratée.
	ServeurMuet EtatServeur = "MUET"
)

// LigneServeur est une ligne du tableau de bord.
type LigneServeur struct {
	Serveur   string             `json:"serveur"`
	Etat      EtatServeur        `json:"etat"`
	RecuLe    time.Time          `json:"recu_le"`
	Silence   time.Duration      `json:"silence_ns"`
	Systeme   string             `json:"systeme"`
	Version   string             `json:"version_agent"`
	Resultats []rapport.Resultat `json:"resultats"`
}

// Synthese est l'état de tout le parc à un instant donné.
type Synthese struct {
	Le       time.Time      `json:"le"`
	Serveurs []LigneServeur `json:"serveurs"`
	OK       int            `json:"ok"`
	Alertes  int            `json:"alertes"`
	Muets    int            `json:"muets"`
}

// Synthetiser classe chaque serveur : muet, en alerte, ou en ordre.
func Synthetiser(derniers []Recu, maintenant time.Time, silenceMax time.Duration) Synthese {
	s := Synthese{Le: maintenant}
	for _, r := range derniers {
		ligne := LigneServeur{
			Serveur:   r.Rapport.Serveur,
			RecuLe:    r.RecuLe,
			Silence:   maintenant.Sub(r.RecuLe),
			Systeme:   r.Rapport.Systeme,
			Version:   r.Rapport.VersionAgent,
			Resultats: r.Rapport.Resultats,
		}
		switch {
		case ligne.Silence > silenceMax:
			ligne.Etat = ServeurMuet
			s.Muets++
		case r.Rapport.Alertes() > 0:
			ligne.Etat = ServeurAlerte
			s.Alertes++
		default:
			ligne.Etat = ServeurOK
			s.OK++
		}
		s.Serveurs = append(s.Serveurs, ligne)
	}
	return s
}

// Texte rédige le rapport du matin : d'abord ce qui demande une action, puis le reste en une ligne.
func (s Synthese) Texte() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sauvegardes au %s : %d serveur(s) en ordre, %d en alerte, %d muet(s).\n",
		s.Le.Format("02/01/2006 15:04"), s.OK, s.Alertes, s.Muets)
	if s.Alertes+s.Muets == 0 {
		b.WriteString("Rien à signaler.\n")
		return b.String()
	}
	for _, l := range s.Serveurs {
		switch l.Etat {
		case ServeurMuet:
			fmt.Fprintf(&b, "\n%s : MUET, aucun rapport depuis %s. L'agent ou le serveur est peut-être arrêté.\n",
				l.Serveur, controle.DureeLisible(l.Silence))
		case ServeurAlerte:
			fmt.Fprintf(&b, "\n%s :\n", l.Serveur)
			for _, r := range l.Resultats {
				if r.Statut.EnAlerte() {
					fmt.Fprintf(&b, "  - %s : %s, %s\n", r.Nom, r.Statut, r.Detail)
				}
			}
		}
	}
	return b.String()
}
