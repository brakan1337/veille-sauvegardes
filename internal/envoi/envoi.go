// Package envoi transmet les rapports au collecteur, avec des nouvelles tentatives
// et une file d'attente locale quand le collecteur est injoignable.
package envoi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brakan1337/veille-sauvegardes/internal/rapport"
)

// MaxEnAttente : au-delà, les plus vieux rapports en attente sont abandonnés.
const MaxEnAttente = 30

// Client envoie les rapports d'un agent.
type Client struct {
	URL        string
	Jeton      string
	HTTP       *http.Client
	Tentatives int
	Pause      time.Duration // doublée à chaque nouvelle tentative
	Attente    string        // fichier des rapports qui n'ont pas pu partir
}

// ErreurDefinitive : le collecteur a refusé le rapport (jeton faux, rapport invalide). Réessayer ne servirait à rien.
type ErreurDefinitive struct {
	Code    int
	Message string
}

func (e ErreurDefinitive) Error() string {
	return fmt.Sprintf("refusé par le collecteur (%d) : %s", e.Code, e.Message)
}

// Envoyer transmet un rapport. En cas d'échec réseau, il est mis en attente pour le prochain passage.
func (c Client) Envoyer(ctx context.Context, r rapport.Rapport) error {
	err := c.transmettre(ctx, r)
	var definitive ErreurDefinitive
	if err != nil && !errors.As(err, &definitive) && c.Attente != "" {
		if errAttente := c.mettreEnAttente(r); errAttente != nil {
			return errors.Join(err, errAttente)
		}
		return fmt.Errorf("%w (rapport mis en attente)", err)
	}
	return err
}

// Rattraper renvoie les rapports en attente, du plus ancien au plus récent.
// Ceux qui échouent encore restent dans le fichier.
func (c Client) Rattraper(ctx context.Context) (envoyes int, err error) {
	rapports, err := c.lireAttente()
	if err != nil || len(rapports) == 0 {
		return 0, err
	}
	var restants []rapport.Rapport
	for i, r := range rapports {
		if err := c.transmettre(ctx, r); err != nil {
			var definitive ErreurDefinitive
			if errors.As(err, &definitive) {
				continue // jamais acceptable : on l'abandonne
			}
			restants = rapports[i:]
			break
		}
		envoyes++
	}
	return envoyes, c.ecrireAttente(restants)
}

func (c Client) transmettre(ctx context.Context, r rapport.Rapport) error {
	corps, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tentatives := max(c.Tentatives, 1)
	pause := c.Pause
	var derniere error
	for essai := 1; essai <= tentatives; essai++ {
		derniere = c.unEssai(ctx, corps)
		var definitive ErreurDefinitive
		if derniere == nil || errors.As(derniere, &definitive) {
			return derniere
		}
		if essai < tentatives {
			select {
			case <-time.After(pause):
				pause *= 2
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return fmt.Errorf("collecteur injoignable après %d tentatives : %w", tentatives, derniere)
}

func (c Client) unEssai(ctx context.Context, corps []byte) error {
	requete, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/api/rapports", bytes.NewReader(corps))
	if err != nil {
		return err
	}
	requete.Header.Set("Content-Type", "application/json")
	requete.Header.Set("Authorization", "Bearer "+c.Jeton)
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	reponse, err := client.Do(requete)
	if err != nil {
		return err
	}
	defer reponse.Body.Close()
	message, _ := io.ReadAll(io.LimitReader(reponse.Body, 2048))
	switch {
	case reponse.StatusCode < 300:
		return nil
	case reponse.StatusCode >= 400 && reponse.StatusCode < 500 && reponse.StatusCode != http.StatusTooManyRequests:
		return ErreurDefinitive{reponse.StatusCode, strings.TrimSpace(string(message))}
	default:
		return fmt.Errorf("réponse %d du collecteur", reponse.StatusCode)
	}
}

func (c Client) mettreEnAttente(r rapport.Rapport) error {
	rapports, err := c.lireAttente()
	if err != nil {
		return err
	}
	rapports = append(rapports, r)
	if len(rapports) > MaxEnAttente {
		rapports = rapports[len(rapports)-MaxEnAttente:]
	}
	return c.ecrireAttente(rapports)
}

func (c Client) lireAttente() ([]rapport.Rapport, error) {
	fichier, err := os.Open(c.Attente)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer fichier.Close()
	var rapports []rapport.Rapport
	lecteur := bufio.NewScanner(fichier)
	lecteur.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for lecteur.Scan() {
		var r rapport.Rapport
		if json.Unmarshal(lecteur.Bytes(), &r) == nil {
			rapports = append(rapports, r)
		}
	}
	return rapports, lecteur.Err()
}

// ecrireAttente remplace le fichier d'un coup (écriture puis renommage) : une coupure ne le laisse jamais à moitié écrit.
func (c Client) ecrireAttente(rapports []rapport.Rapport) error {
	if len(rapports) == 0 {
		err := os.Remove(c.Attente)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.Attente), 0o750); err != nil {
		return err
	}
	var tampon bytes.Buffer
	for _, r := range rapports {
		ligne, _ := json.Marshal(r)
		tampon.Write(ligne)
		tampon.WriteByte('\n')
	}
	temporaire := c.Attente + ".tmp"
	if err := os.WriteFile(temporaire, tampon.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(temporaire, c.Attente)
}
