variable "client" {
  description = "Identifiant court du client, utilisé dans les noms et l'adresse (ex. boulangerie-dupuy)."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$", var.client))
    error_message = "Le client s'écrit en minuscules, chiffres et tirets (32 caractères au plus), sans tiret au bord."
  }
}

variable "domaine" {
  description = "Domaine de l'infogérant ; le collecteur répond sur collecte.<client>.<domaine>."
  type        = string
}

variable "zone_dns_id" {
  description = "Identifiant de la zone Route 53 qui porte le domaine."
  type        = string
}

variable "url_binaire" {
  description = "Adresse https du binaire du collecteur pour Linux ARM64 (une release GitHub)."
  type        = string

  validation {
    condition     = startswith(var.url_binaire, "https://")
    error_message = "Le binaire se télécharge en https uniquement."
  }
}

variable "type_instance" {
  description = "Type de machine. Un collecteur sert quelques dizaines de serveurs : la plus petite suffit."
  type        = string
  default     = "t4g.micro"

  validation {
    condition     = can(regex("^t4g\\.", var.type_instance))
    error_message = "Le binaire publié est compilé pour ARM64 : choisissez un type t4g."
  }
}

variable "cidr_admin" {
  description = "Adresses autorisées en SSH (le réseau de l'infogérant). Vide : aucun accès SSH."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for c in var.cidr_admin : can(cidrhost(c, 0)) && c != "0.0.0.0/0"])
    error_message = "Chaque entrée doit être un CIDR valide, et SSH ne s'ouvre jamais à tout Internet."
  }
}

variable "heure_rapport" {
  description = "Heure du rapport du matin, heure de Paris."
  type        = string
  default     = "07:30"

  validation {
    condition     = can(regex("^([01][0-9]|2[0-3]):[0-5][0-9]$", var.heure_rapport))
    error_message = "Format attendu : HH:MM."
  }
}

variable "webhook" {
  description = "Webhook où publier le rapport du matin (Slack, Mattermost, Teams). Vide : pas de publication."
  type        = string
  default     = ""
  sensitive   = true
}

variable "taille_disque_go" {
  description = "Taille du disque système, en Go."
  type        = number
  default     = 10
}
