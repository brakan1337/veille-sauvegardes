output "url" {
  description = "Adresse à donner aux agents de ce client (--collecteur)."
  value       = "https://${aws_route53_record.collecteur.name}"
}

output "ip" {
  description = "Adresse IP fixe du collecteur."
  value       = aws_eip.collecteur.public_ip
}

output "jeton_agents" {
  description = "Jeton à donner aux agents de ce client (--jeton)."
  value       = random_password.jeton_agents.result
  sensitive   = true
}

output "mot_de_passe_admin" {
  description = "Mot de passe du tableau de bord (utilisateur admin)."
  value       = random_password.mot_de_passe_admin.result
  sensitive   = true
}

output "script_demarrage" {
  description = "Script exécuté au premier démarrage. Il ne contient aucun secret : il les lit dans le coffre."
  value       = local.script_demarrage
}
