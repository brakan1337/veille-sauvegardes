output "collecteurs" {
  description = "Adresse et IP du collecteur de chaque client."
  value       = { for client, c in module.collecteur : client => { url = c.url, ip = c.ip } }
}

output "jetons_agents" {
  description = "Jeton des agents de chaque client : terraform output -json jetons_agents"
  value       = { for client, c in module.collecteur : client => c.jeton_agents }
  sensitive   = true
}

output "mots_de_passe_admin" {
  description = "Mot de passe du tableau de bord de chaque client."
  value       = { for client, c in module.collecteur : client => c.mot_de_passe_admin }
  sensitive   = true
}
