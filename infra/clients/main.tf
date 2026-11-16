# Le parc de l'infogérant : un collecteur par client.
# Ajouter une PME = ajouter une ligne dans terraform.tfvars, puis `terraform apply`.

terraform {
  required_version = ">= 1.7"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }

  # En équipe, l'état se partage dans un bucket S3 verrouillé :
  # backend "s3" {
  #   bucket       = "infogerance-etat-terraform"
  #   key          = "veille/clients.tfstate"
  #   region       = "eu-west-3"
  #   use_lockfile = true
  # }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Projet  = "veille-sauvegardes"
      GerePar = "terraform"
    }
  }
}

module "collecteur" {
  source   = "../modules/collecteur"
  for_each = var.clients

  client        = each.key
  domaine       = var.domaine
  zone_dns_id   = var.zone_dns_id
  url_binaire   = "https://github.com/brakan1337/veille-sauvegardes/releases/download/${var.version_collecteur}/veille-collecteur-linux-arm64"
  cidr_admin    = var.cidr_admin
  type_instance = each.value.type_instance
  heure_rapport = each.value.heure_rapport
  webhook       = each.value.webhook
}
