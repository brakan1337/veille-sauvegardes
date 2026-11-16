// Le collecteur : un par client (mono-tenant). Il reçoit les rapports des agents,
// affiche le tableau de bord et publie la synthèse chaque matin.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // le fuseau Europe/Paris fonctionne même sur une image sans base de fuseaux

	"github.com/brakan1337/veille-sauvegardes/internal/collecte"
	"github.com/brakan1337/veille-sauvegardes/internal/controle"
)

var version = "dev"

func variable(nom, defaut string) string {
	if v := os.Getenv(nom); v != "" {
		return v
	}
	return defaut
}

func main() {
	journal := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := executer(journal); err != nil {
		journal.Error("arrêt", "erreur", err)
		os.Exit(1)
	}
}

func executer(journal *slog.Logger) error {
	if len(os.Args) > 1 && os.Args[1] == "-version" {
		fmt.Println("veille-collecteur", version)
		return nil
	}
	jeton := os.Getenv("VEILLE_JETON_AGENTS")
	motDePasse := os.Getenv("VEILLE_MOT_DE_PASSE_ADMIN")
	if len(jeton) < 24 || len(motDePasse) < 12 {
		return errors.New("VEILLE_JETON_AGENTS (24 caractères min.) et VEILLE_MOT_DE_PASSE_ADMIN (12 min.) sont obligatoires")
	}
	silence, err := controle.LireDuree(variable("VEILLE_SILENCE_MAX", "26h"))
	if err != nil {
		return err
	}
	fuseau, err := time.LoadLocation(variable("VEILLE_FUSEAU", "Europe/Paris"))
	if err != nil {
		return err
	}

	depot, err := collecte.Ouvrir(variable("VEILLE_DONNEES", "donnees/rapports.jsonl"))
	if err != nil {
		return err
	}
	defer depot.Fermer()

	serveur := &collecte.Serveur{
		Depot: depot, JetonAgents: jeton, MotDePasseAdmin: motDePasse,
		SilenceMax: silence, Horloge: time.Now, Journal: journal,
	}

	ctx, arreter := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer arreter()

	if webhook := os.Getenv("VEILLE_WEBHOOK"); webhook != "" {
		go func() {
			if err := serveur.PublierChaqueMatin(ctx, webhook, variable("VEILLE_HEURE_RAPPORT", "07:30"), fuseau); err != nil {
				journal.Error("rapport du matin désactivé", "erreur", err)
			}
		}()
	}

	ecoute := &http.Server{
		Addr:              variable("VEILLE_ADRESSE", ":8080"),
		Handler:           serveur.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		fin, annuler := context.WithTimeout(context.Background(), 10*time.Second)
		defer annuler()
		ecoute.Shutdown(fin)
	}()
	journal.Info("collecteur démarré", "adresse", ecoute.Addr, "version", version)
	if err := ecoute.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
