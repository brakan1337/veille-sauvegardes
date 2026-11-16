// L'agent : un seul fichier exécutable, posé sur chaque serveur surveillé.
// Il contrôle les sauvegardes, envoie le rapport au collecteur et s'arrête.
// C'est le planificateur du système (minuterie systemd, tâche Windows) qui le relance.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/agent"
	"github.com/brakan1337/veille-sauvegardes/internal/controle"
)

// version est remplie à la compilation : -ldflags "-X main.version=v1.2.0".
var version = "dev"

func main() {
	os.Exit(executer())
}

func cheminParDefaut() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("ProgramData") + `\veille\agent.json`
	}
	return "/etc/veille/agent.json"
}

func executer() int {
	config := flag.String("config", cheminParDefaut(), "fichier de configuration")
	verifierSeulement := flag.Bool("verifier", false, "afficher le résultat des contrôles sans rien envoyer")
	afficherVersion := flag.Bool("version", false, "afficher la version")
	flag.Parse()

	if *afficherVersion {
		fmt.Println("veille-agent", version)
		return 0
	}

	cfg, err := controle.Charger(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration :", err)
		return 2
	}

	ctx, arreter := signal.NotifyContext(context.Background(), os.Interrupt)
	defer arreter()

	r := agent.Controler(ctx, cfg, version, time.Now())
	for _, res := range r.Resultats {
		fmt.Printf("%-9s %-24s %s\n", res.Statut, res.Nom, res.Detail)
	}
	if *verifierSeulement {
		if r.Alertes() > 0 {
			return 1
		}
		return 0
	}

	client := agent.Client(cfg)
	if n, err := client.Rattraper(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "rapports en attente :", err)
	} else if n > 0 {
		fmt.Printf("%d rapport(s) en attente rattrapé(s)\n", n)
	}
	if err := client.Envoyer(ctx, r); err != nil {
		fmt.Fprintln(os.Stderr, "envoi :", err)
		return 3
	}
	fmt.Println("rapport envoyé à", cfg.Collecteur)
	return 0
}
