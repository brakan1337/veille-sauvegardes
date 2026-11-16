variable "region" {
  description = "Région AWS (Paris par défaut)."
  type        = string
  default     = "eu-west-3"
}

variable "domaine" {
  description = "Domaine de l'infogérant."
  type        = string
}

variable "zone_dns_id" {
  description = "Zone Route 53 du domaine."
  type        = string
}

variable "cidr_admin" {
  description = "Réseau de l'infogérant, seul autorisé en SSH."
  type        = list(string)
  default     = []
}

variable "version_collecteur" {
  description = "Release GitHub du collecteur à installer (ex. v1.0.0)."
  type        = string
}

variable "clients" {
  description = "Un collecteur par client. La clé est l'identifiant court du client."
  type = map(object({
    type_instance = optional(string, "t4g.micro")
    heure_rapport = optional(string, "07:30")
    webhook       = optional(string, "")
  }))
}
