// apercu fait produire au vrai collecteur son tableau de bord pour un parc d'exemple,
// et écrit la page sur la sortie standard. Il sert aux captures du README.
//
//	go run ./outils/apercu > apercu.html
package main

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/collecte"
	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

func main() {
	maintenant := time.Now()
	dossier, err := os.MkdirTemp("", "apercu")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dossier)
	depot, err := collecte.Ouvrir(filepath.Join(dossier, "rapports.jsonl"))
	if err != nil {
		panic(err)
	}
	defer depot.Fermer()

	envoyer := func(serveur, systeme string, il_y_a time.Duration, resultats ...rapport.Resultat) {
		depot.Enregistrer(collecte.Recu{RecuLe: maintenant.Add(-il_y_a), Rapport: rapport.Rapport{
			Serveur: serveur, Systeme: systeme, VersionAgent: "v1.0.0", EmisLe: maintenant.Add(-il_y_a), Resultats: resultats,
		}})
	}
	res := func(nom, chemin string, statut rapport.Statut, detail, fichier string) rapport.Resultat {
		return rapport.Resultat{Nom: nom, Chemin: chemin, Statut: statut, Detail: detail, Fichier: fichier}
	}

	envoyer("srv-compta", "linux/amd64", 2*time.Hour,
		res("Base comptable", "/var/backups/compta", rapport.EnChute, "126,0 Mo contre 4,0 Go la fois précédente", "compta-20261116.sql.gz"),
		res("Export de paie", "/var/backups/paie", rapport.OK, "820,0 Mo, il y a 5 h", "paie-20261116.zip"))
	envoyer("srv-fichiers", "windows/amd64", 90*time.Minute,
		res("Partages réseau", `D:\Sauvegardes\Partages`, rapport.OK, "58,2 Go, il y a 5 h", "partages-20261116.vbk"))
	envoyer("srv-impression", "windows/amd64", 50*time.Hour,
		res("Configuration des imprimantes", `D:\Sauvegardes\Impression`, rapport.OK, "2,1 Mo, il y a 5 h", "impression-20261114.zip"))
	envoyer("srv-messagerie", "linux/arm64", time.Hour,
		res("Boîtes mail", "/srv/sauvegardes/mail", rapport.Ancienne, "dernière sauvegarde il y a 4 j 4 h (maximum 1 j 2 h)", "mail-20261112.tar.zst"),
		res("Annuaire", "/srv/sauvegardes/annuaire", rapport.Absente, "aucun fichier « *.ldif » dans le dossier", ""))

	serveur := &collecte.Serveur{
		Depot: depot, JetonAgents: "apercu", MotDePasseAdmin: "apercu", SilenceMax: 26 * time.Hour,
		Horloge: func() time.Time { return maintenant }, Journal: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	requete := httptest.NewRequest("GET", "/", nil)
	requete.SetBasicAuth("admin", "apercu")
	reponse := httptest.NewRecorder()
	serveur.Routes().ServeHTTP(reponse, requete)
	os.Stdout.Write(reponse.Body.Bytes())
}
