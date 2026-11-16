# Tests du module sans compte AWS : les fournisseurs sont simulés (mock_provider),
# Terraform calcule le plan et l'application comme s'ils étaient réels.

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = { region = "eu-west-3" }
  }
  mock_data "aws_ami" {
    defaults = { id = "ami-0123456789abcdef0" }
  }
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}" }
  }
  mock_resource "aws_ssm_parameter" {
    defaults = { arn = "arn:aws:ssm:eu-west-3:111122223333:parameter/veille/test" }
  }
  mock_resource "aws_eip" {
    defaults = { public_ip = "203.0.113.10" }
  }
}

mock_provider "random" {}

variables {
  client      = "boulangerie-dupuy"
  domaine     = "veille.infogerance.example"
  zone_dns_id = "Z0123456789ABCDEFGHIJ"
  url_binaire = "https://github.com/brakan1337/veille-sauvegardes/releases/download/v1.0.0/veille-collecteur-linux-arm64"
  cidr_admin  = ["198.51.100.0/24"]
}

run "nommage_et_adresse" {
  command = apply

  assert {
    condition     = output.url == "https://collecte.boulangerie-dupuy.veille.infogerance.example"
    error_message = "L'adresse du collecteur doit suivre collecte.<client>.<domaine>."
  }
  assert {
    condition     = aws_instance.collecteur.tags["Name"] == "veille-boulangerie-dupuy" && aws_instance.collecteur.tags["Client"] == "boulangerie-dupuy"
    error_message = "La machine doit porter le nom et le client dans ses étiquettes."
  }
  assert {
    condition     = aws_route53_record.collecteur.records == toset(["203.0.113.10"])
    error_message = "Le nom de domaine doit pointer vers l'IP fixe."
  }
}

run "securite_de_la_machine" {
  command = apply

  assert {
    condition     = aws_instance.collecteur.metadata_options[0].http_tokens == "required"
    error_message = "IMDSv2 doit être obligatoire."
  }
  assert {
    condition     = aws_instance.collecteur.root_block_device[0].encrypted
    error_message = "Le disque doit être chiffré."
  }
  assert {
    condition     = length(aws_vpc_security_group_ingress_rule.ssh) == 1 && aws_vpc_security_group_ingress_rule.ssh["198.51.100.0/24"].from_port == 22
    error_message = "SSH ne s'ouvre qu'aux adresses de l'infogérant."
  }
  assert {
    condition     = toset(keys(aws_vpc_security_group_ingress_rule.web)) == toset(["80", "443"])
    error_message = "Seuls 80 et 443 sont ouverts au public."
  }
}

run "aucun_secret_dans_le_script_de_demarrage" {
  command = apply

  assert {
    condition     = !strcontains(output.script_demarrage, random_password.jeton_agents.result)
    error_message = "Le jeton des agents ne doit pas apparaître dans le script de démarrage."
  }
  assert {
    condition     = strcontains(output.script_demarrage, "/veille/boulangerie-dupuy/jeton-agents")
    error_message = "Le script doit lire le jeton dans le coffre, sous le nom propre à ce client."
  }
  assert {
    condition     = !strcontains(output.script_demarrage, "VEILLE_WEBHOOK")
    error_message = "Sans webhook, le script ne doit pas en configurer."
  }
}

run "avec_webhook" {
  command = apply

  variables {
    webhook = "https://hooks.example/services/T000/B000/xxx"
  }

  assert {
    condition     = length(aws_ssm_parameter.webhook) == 1 && strcontains(output.script_demarrage, "/veille/boulangerie-dupuy/webhook")
    error_message = "Le webhook doit être rangé dans le coffre et lu au démarrage."
  }
  assert {
    condition     = !strcontains(output.script_demarrage, "hooks.example")
    error_message = "L'adresse du webhook ne doit pas apparaître en clair dans le script."
  }
}

run "sans_acces_ssh" {
  command = plan

  variables {
    cidr_admin = []
  }

  assert {
    condition     = length(aws_vpc_security_group_ingress_rule.ssh) == 0
    error_message = "Sans adresse d'administration, aucun accès SSH."
  }
}

run "client_mal_ecrit" {
  command = plan

  variables {
    client = "Boulangerie Dupuy"
  }

  expect_failures = [var.client]
}

run "ssh_ouvert_a_tout_internet_refuse" {
  command = plan

  variables {
    cidr_admin = ["0.0.0.0/0"]
  }

  expect_failures = [var.cidr_admin]
}

run "type_de_machine_incompatible" {
  command = plan

  variables {
    type_instance = "t3.micro"
  }

  expect_failures = [var.type_instance]
}
