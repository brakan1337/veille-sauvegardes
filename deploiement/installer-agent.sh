#!/usr/bin/env bash
# Installe (ou met à jour) l'agent de veille des sauvegardes sur un serveur Linux.
#
#   sudo ./installer-agent.sh --collecteur https://collecte.client.example \
#        --jeton "$JETON" --binaire ./veille-agent-linux-amd64 [--config sauvegardes.json]
#
# Le script peut être relancé sans risque : il remplace le binaire, garde la
# configuration existante sauf si --config est fourni, et réactive la minuterie.
# L'agent tourne chaque matin sous un utilisateur système sans droits de connexion.

set -Eeuo pipefail

readonly UTILISATEUR="veille"
readonly BINAIRE_CIBLE="/usr/local/bin/veille-agent"
readonly DOSSIER_CONFIG="/etc/veille"
readonly DOSSIER_ETAT="/var/lib/veille"
readonly UNITE="veille-agent"

collecteur=""
jeton=""
binaire=""
url=""
config=""
heure="06:30"
lecture=()
temporaire=""

# Le fichier temporaire disparaît même si le script s'arrête en route.
trap 'rm -f -- "${temporaire:-}"' EXIT

usage() {
  cat <<'FIN'
Usage : installer-agent.sh --collecteur URL --jeton JETON (--binaire FICHIER | --url URL) [options]

  --collecteur URL   adresse du collecteur du client (https://…)
  --jeton JETON      jeton des agents de ce client
  --binaire FICHIER  agent déjà téléchargé
  --url URL          ou adresse où télécharger l'agent
  --config FICHIER   liste des sauvegardes à surveiller (JSON) ; sinon un modèle à compléter
  --heure HH:MM      heure du contrôle quotidien (défaut 06:30)
  --lecture DOSSIER  dossier de sauvegardes que l'agent doit pouvoir lire (répétable)
  --aide             afficher cette aide
FIN
}

erreur() {
  echo "Erreur : $*" >&2
  exit 1
}

info() {
  echo "==> $*"
}

lire_arguments() {
  while (($# > 0)); do
    case "$1" in
      --collecteur) collecteur="${2:?}"; shift 2 ;;
      --jeton) jeton="${2:?}"; shift 2 ;;
      --binaire) binaire="${2:?}"; shift 2 ;;
      --url) url="${2:?}"; shift 2 ;;
      --config) config="${2:?}"; shift 2 ;;
      --heure) heure="${2:?}"; shift 2 ;;
      --lecture) lecture+=("${2:?}"); shift 2 ;;
      --aide | -h) usage; exit 0 ;;
      *) usage >&2; erreur "option inconnue : $1" ;;
    esac
  done

  [[ $collecteur =~ ^https?:// ]] || erreur "--collecteur doit être une adresse http(s)"
  # Le jeton et l'adresse sont écrits dans un fichier JSON : aucun guillemet ni caractère d'échappement.
  [[ $jeton =~ ^[A-Za-z0-9._~+/=-]{24,}$ ]] || erreur "--jeton doit faire 24 caractères au moins, sans espace ni guillemet"
  [[ $collecteur =~ ^https?://[A-Za-z0-9.:/_-]+$ ]] || erreur "--collecteur contient des caractères inattendus"
  [[ -n $binaire || -n $url ]] || erreur "--binaire ou --url est obligatoire"
  [[ $heure =~ ^([01][0-9]|2[0-3]):[0-5][0-9]$ ]] || erreur "--heure attend le format HH:MM"
  if [[ -n $config && ! -r $config ]]; then
    erreur "configuration illisible : $config"
  fi
  return 0
}

verifier_systeme() {
  ((EUID == 0)) || erreur "lancez ce script avec sudo"
  command -v systemctl >/dev/null || erreur "systemd est nécessaire"
  case "$(uname -m)" in
    x86_64 | aarch64) ;;
    *) erreur "architecture non prise en charge : $(uname -m)" ;;
  esac
}

installer_binaire() {
  temporaire="$(mktemp)"

  if [[ -n $url ]]; then
    info "Téléchargement de l'agent"
    curl --fail --silent --show-error --location --proto '=https' --output "$temporaire" "$url"
  else
    cp -- "$binaire" "$temporaire"
  fi
  chmod 0755 "$temporaire"
  "$temporaire" -version >/dev/null || erreur "le binaire ne s'exécute pas sur ce serveur"
  install -o root -g root -m 0755 "$temporaire" "$BINAIRE_CIBLE"
  info "Agent installé : $("$BINAIRE_CIBLE" -version)"
}

creer_utilisateur() {
  if ! id -u "$UTILISATEUR" >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin "$UTILISATEUR"
    info "Utilisateur système « $UTILISATEUR » créé"
  fi
  install -d -o "$UTILISATEUR" -g "$UTILISATEUR" -m 0750 "$DOSSIER_ETAT"
  install -d -o root -g "$UTILISATEUR" -m 0750 "$DOSSIER_CONFIG"
}

ecrire_configuration() {
  local cible="$DOSSIER_CONFIG/agent.json"
  local sauvegardes

  if [[ -n $config ]]; then
    sauvegardes="$(cat -- "$config")"
  elif [[ -f $cible ]]; then
    info "Configuration existante conservée : $cible"
    return 0
  else
    sauvegardes='[{"nom": "À COMPLÉTER", "chemin": "/var/backups", "motif": "*.tar.gz", "age_max": "26h", "taille_min": "1Mo"}]'
    info "Modèle de configuration écrit : complétez la liste des sauvegardes dans $cible"
  fi

  # Le jeton reste lisible par root et l'agent seulement.
  umask 027
  cat >"$cible" <<FIN
{
  "collecteur": "$collecteur",
  "jeton": "$jeton",
  "serveur": "$(hostname -s)",
  "attente": "$DOSSIER_ETAT/attente.jsonl",
  "sauvegardes": $sauvegardes
}
FIN
  chown root:"$UTILISATEUR" "$cible"
  chmod 0640 "$cible"

  # L'agent sort avec le code 2 quand la configuration est invalide (1 = sauvegardes en alerte, sans gravité ici).
  local code=0
  "$BINAIRE_CIBLE" -config "$cible" -verifier >/dev/null 2>&1 || code=$?
  ((code != 2)) || erreur "configuration invalide, voir : $BINAIRE_CIBLE -config $cible -verifier"
}

donner_acces_lecture() {
  local dossier
  for dossier in "${lecture[@]}"; do
    [[ -d $dossier ]] || erreur "dossier introuvable : $dossier"
    if command -v setfacl >/dev/null; then
      setfacl -R -m "u:$UTILISATEUR:rX" "$dossier"
      setfacl -d -m "u:$UTILISATEUR:rX" "$dossier"
      info "Lecture accordée à « $UTILISATEUR » sur $dossier"
    else
      echo "Attention : setfacl absent, donnez vous-même à « $UTILISATEUR » le droit de lire $dossier" >&2
    fi
  done
}

installer_minuterie() {
  cat >"/etc/systemd/system/$UNITE.service" <<FIN
[Unit]
Description=Contrôle des sauvegardes et envoi du rapport au collecteur
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
User=$UTILISATEUR
Group=$UTILISATEUR
ExecStart=$BINAIRE_CIBLE -config $DOSSIER_CONFIG/agent.json
# L'agent ne fait que lire : on lui retire tout le reste.
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=$DOSSIER_ETAT
PrivateTmp=yes
FIN

  cat >"/etc/systemd/system/$UNITE.timer" <<FIN
[Unit]
Description=Contrôle quotidien des sauvegardes

[Timer]
OnCalendar=*-*-* $heure:00
# Étale les envois quand plusieurs serveurs du même client démarrent ensemble.
RandomizedDelaySec=10min
Persistent=true

[Install]
WantedBy=timers.target
FIN

  systemctl daemon-reload
  systemctl enable --now "$UNITE.timer" >/dev/null
  info "Contrôle programmé chaque jour vers $heure ($(systemctl show -P NextElapseUSecRealtime "$UNITE.timer"))"
}

main() {
  lire_arguments "$@"
  verifier_systeme
  installer_binaire
  creer_utilisateur
  ecrire_configuration
  donner_acces_lecture
  installer_minuterie
  info "Terminé. Essai immédiat : sudo systemctl start $UNITE.service && journalctl -u $UNITE -n 20"
}

main "$@"
