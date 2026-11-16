# Un collecteur pour un client : sa machine, son adresse, ses secrets.
# Rien n'est partagé entre deux clients (mono-tenant) : une fuite ou une panne
# chez l'un ne touche jamais les autres.

locals {
  nom         = "veille-${var.client}"
  nom_domaine = "collecte.${var.client}.${var.domaine}"
  prefixe_ssm = "/veille/${var.client}"
  tags        = { Client = var.client, Role = "collecteur" }

  # Savoir s'il y a un webhook n'est pas un secret ; son adresse, si.
  avec_webhook = nonsensitive(var.webhook != "")

  script_demarrage = templatefile("${path.module}/demarrage.sh.tftpl", {
    nom_domaine     = local.nom_domaine
    url_binaire     = var.url_binaire
    region          = data.aws_region.courante.region
    parametre_jeton = aws_ssm_parameter.jeton_agents.name
    parametre_admin = aws_ssm_parameter.mot_de_passe_admin.name
    parametre_hook  = local.avec_webhook ? aws_ssm_parameter.webhook[0].name : ""
    heure_rapport   = var.heure_rapport
  })
}

data "aws_region" "courante" {}

data "aws_ami" "debian" {
  most_recent = true
  owners      = ["136693071363"] # le compte officiel de Debian

  filter {
    name   = "name"
    values = ["debian-12-arm64-*"]
  }
}

# --- Secrets : générés ici, rangés dans le coffre, lus par la machine au démarrage ---

resource "random_password" "jeton_agents" {
  length  = 40
  special = false
}

resource "random_password" "mot_de_passe_admin" {
  length  = 24
  special = false
}

resource "aws_ssm_parameter" "jeton_agents" {
  name  = "${local.prefixe_ssm}/jeton-agents"
  type  = "SecureString"
  value = random_password.jeton_agents.result
  tags  = local.tags
}

resource "aws_ssm_parameter" "mot_de_passe_admin" {
  name  = "${local.prefixe_ssm}/mot-de-passe-admin"
  type  = "SecureString"
  value = random_password.mot_de_passe_admin.result
  tags  = local.tags
}

resource "aws_ssm_parameter" "webhook" {
  count = local.avec_webhook ? 1 : 0
  name  = "${local.prefixe_ssm}/webhook"
  type  = "SecureString"
  value = var.webhook
  tags  = local.tags
}

# --- Droits de la machine : lire ses propres secrets, rien d'autre ---

data "aws_iam_policy_document" "assumer" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "lire_secrets" {
  statement {
    actions = ["ssm:GetParameter"]
    resources = concat(
      [aws_ssm_parameter.jeton_agents.arn, aws_ssm_parameter.mot_de_passe_admin.arn],
      aws_ssm_parameter.webhook[*].arn,
    )
  }
}

resource "aws_iam_role" "collecteur" {
  name               = local.nom
  assume_role_policy = data.aws_iam_policy_document.assumer.json
  tags               = local.tags
}

resource "aws_iam_role_policy" "lire_secrets" {
  name   = "lire-ses-secrets"
  role   = aws_iam_role.collecteur.id
  policy = data.aws_iam_policy_document.lire_secrets.json
}

resource "aws_iam_instance_profile" "collecteur" {
  name = local.nom
  role = aws_iam_role.collecteur.name
}

# --- Réseau : https pour les agents, SSH seulement depuis l'infogérant ---

resource "aws_security_group" "collecteur" {
  name        = local.nom
  description = "Collecteur ${var.client} : https public, SSH restreint"
  tags        = local.tags
}

resource "aws_vpc_security_group_ingress_rule" "web" {
  for_each          = toset(["80", "443"]) # 80 sert au certificat Let's Encrypt, puis redirige vers 443
  security_group_id = aws_security_group.collecteur.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = tonumber(each.value)
  to_port           = tonumber(each.value)
}

resource "aws_vpc_security_group_ingress_rule" "ssh" {
  for_each          = toset(var.cidr_admin)
  security_group_id = aws_security_group.collecteur.id
  cidr_ipv4         = each.value
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
}

resource "aws_vpc_security_group_egress_rule" "sortie" {
  security_group_id = aws_security_group.collecteur.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

# --- La machine ---

resource "aws_instance" "collecteur" {
  ami                    = data.aws_ami.debian.id
  instance_type          = var.type_instance
  iam_instance_profile   = aws_iam_instance_profile.collecteur.name
  vpc_security_group_ids = [aws_security_group.collecteur.id]
  user_data              = local.script_demarrage

  # Une nouvelle version du script de démarrage reconstruit la machine plutôt que de la laisser à moitié à jour.
  user_data_replace_on_change = true

  metadata_options {
    http_tokens = "required" # IMDSv2 : protège les identifiants de la machine contre les requêtes détournées
  }

  root_block_device {
    volume_type = "gp3"
    volume_size = var.taille_disque_go
    encrypted   = true
  }

  tags = merge(local.tags, { Name = local.nom })

  lifecycle {
    ignore_changes = [ami] # une nouvelle image Debian ne doit pas reconstruire la machine en silence
  }
}

resource "aws_eip" "collecteur" {
  instance = aws_instance.collecteur.id
  domain   = "vpc"
  tags     = merge(local.tags, { Name = local.nom })
}

resource "aws_route53_record" "collecteur" {
  zone_id = var.zone_dns_id
  name    = local.nom_domaine
  type    = "A"
  ttl     = 300
  records = [aws_eip.collecteur.public_ip]
}
