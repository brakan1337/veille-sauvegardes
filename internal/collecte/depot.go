// Package collecte reçoit les rapports des agents, garde le dernier de chaque
// serveur et en tire la synthèse affichée chaque matin.
package collecte

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

// Recu : un rapport tel qu'il a été reçu, avec l'heure d'arrivée au collecteur.
type Recu struct {
	RecuLe  time.Time       `json:"recu_le"`
	Rapport rapport.Rapport `json:"rapport"`
}

// Depot garde l'historique dans un fichier JSON Lines (une ligne par rapport, en ajout seul)
// et le dernier rapport de chaque serveur en mémoire.
//
// Un collecteur sert un seul client (mono-tenant) : quelques dizaines de serveurs,
// un rapport par jour chacun. Un fichier suffit ; pas besoin d'une base de données à opérer.
type Depot struct {
	mu       sync.RWMutex
	fichier  *os.File
	derniers map[string]Recu
}

// Ouvrir relit l'historique existant et prépare l'ajout de nouveaux rapports.
func Ouvrir(chemin string) (*Depot, error) {
	if err := os.MkdirAll(filepath.Dir(chemin), 0o750); err != nil {
		return nil, err
	}
	d := &Depot{derniers: map[string]Recu{}}
	if existant, err := os.Open(chemin); err == nil {
		lecteur := bufio.NewScanner(existant)
		lecteur.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for numero := 1; lecteur.Scan(); numero++ {
			var r Recu
			if err := json.Unmarshal(lecteur.Bytes(), &r); err != nil {
				existant.Close()
				return nil, fmt.Errorf("%s, ligne %d : %w", chemin, numero, err)
			}
			d.retenir(r)
		}
		existant.Close()
		if err := lecteur.Err(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	fichier, err := os.OpenFile(chemin, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	d.fichier = fichier
	return d, nil
}

func (d *Depot) retenir(r Recu) {
	if precedent, ok := d.derniers[r.Rapport.Serveur]; ok && precedent.Rapport.EmisLe.After(r.Rapport.EmisLe) {
		return // un rapport rattrapé en retard ne remplace pas un rapport plus récent
	}
	d.derniers[r.Rapport.Serveur] = r
}

// Enregistrer ajoute un rapport à l'historique.
func (d *Depot) Enregistrer(r Recu) error {
	ligne, err := json.Marshal(r)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.fichier.Write(append(ligne, '\n')); err != nil {
		return err
	}
	d.retenir(r)
	return nil
}

// Fermer libère le fichier.
func (d *Depot) Fermer() error { return d.fichier.Close() }

// Derniers renvoie le dernier rapport de chaque serveur, par ordre alphabétique.
func (d *Depot) Derniers() []Recu {
	d.mu.RLock()
	defer d.mu.RUnlock()
	liste := make([]Recu, 0, len(d.derniers))
	for _, r := range d.derniers {
		liste = append(liste, r)
	}
	sort.Slice(liste, func(i, j int) bool { return liste[i].Rapport.Serveur < liste[j].Rapport.Serveur })
	return liste
}
