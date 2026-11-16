package collecte

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ProchainEnvoi : la prochaine occurrence de l'heure donnée (« 07:30 ») dans le fuseau donné.
func ProchainEnvoi(maintenant time.Time, heure string, fuseau *time.Location) (time.Time, error) {
	h, err := time.Parse("15:04", heure)
	if err != nil {
		return time.Time{}, fmt.Errorf("heure invalide %q (format attendu : 07:30)", heure)
	}
	local := maintenant.In(fuseau)
	envoi := time.Date(local.Year(), local.Month(), local.Day(), h.Hour(), h.Minute(), 0, 0, fuseau)
	if !envoi.After(local) {
		envoi = envoi.AddDate(0, 0, 1)
	}
	return envoi, nil
}

// PublierChaqueMatin envoie la synthèse à un webhook (Slack, Mattermost, Teams…)
// chaque jour à l'heure donnée, jusqu'à l'arrêt du collecteur.
func (s *Serveur) PublierChaqueMatin(ctx context.Context, webhook, heure string, fuseau *time.Location) error {
	for {
		prochain, err := ProchainEnvoi(s.Horloge(), heure, fuseau)
		if err != nil {
			return err
		}
		s.Journal.Info("prochain rapport du matin", "le", prochain.Format(time.RFC3339))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Until(prochain)):
		}
		if err := Publier(ctx, webhook, s.Synthese().Texte()); err != nil {
			s.Journal.Error("rapport du matin non envoyé", "erreur", err)
		}
	}
}

// Publier poste un message texte au format compris par la plupart des messageries ({"text": "…"}).
func Publier(ctx context.Context, webhook, texte string) error {
	corps, _ := json.Marshal(map[string]string{"text": texte})
	requete, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(corps))
	if err != nil {
		return err
	}
	requete.Header.Set("Content-Type", "application/json")
	reponse, err := (&http.Client{Timeout: 15 * time.Second}).Do(requete)
	if err != nil {
		return err
	}
	reponse.Body.Close()
	if reponse.StatusCode >= 300 {
		return fmt.Errorf("le webhook a répondu %d", reponse.StatusCode)
	}
	return nil
}
