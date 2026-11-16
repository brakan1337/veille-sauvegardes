// Package agent enchaîne une passe complète : contrôler, rattraper les rapports en attente, envoyer.
package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/controle"
	"github.com/brakan1337/veille-sauvegardes/internal/envoi"
	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

// DelaiParControle : un dossier qui ne répond pas dans ce délai est signalé en erreur.
const DelaiParControle = 30 * time.Second

// Controler produit le rapport de la passe, sans rien envoyer.
func Controler(ctx context.Context, cfg controle.Config, version string, maintenant time.Time) rapport.Rapport {
	return rapport.Rapport{
		Serveur:      cfg.Serveur,
		Systeme:      runtime.GOOS + "/" + runtime.GOARCH,
		VersionAgent: version,
		EmisLe:       maintenant.UTC(),
		Resultats:    controle.VerifierTout(ctx, cfg.Sauvegardes, maintenant, DelaiParControle, controle.Verifier),
	}
}

// Client prépare l'envoi vers le collecteur de la configuration.
func Client(cfg controle.Config) envoi.Client {
	return envoi.Client{
		URL:        cfg.Collecteur,
		Jeton:      cfg.Jeton,
		Tentatives: 4,
		Pause:      5 * time.Second,
		Attente:    FichierAttente(cfg),
	}
}

// FichierAttente : là où l'agent garde les rapports qui n'ont pas pu partir.
func FichierAttente(cfg controle.Config) string {
	if cfg.Attente != "" {
		return cfg.Attente
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "veille", "attente.jsonl")
	}
	return "/var/lib/veille/attente.jsonl"
}
