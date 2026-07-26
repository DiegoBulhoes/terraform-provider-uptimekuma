output "environment_id" {
  description = "ID of the environment tag."
  value       = uptimekuma_tag.environment.id
}

output "team_id" {
  description = "ID of the team tag."
  value       = uptimekuma_tag.team.id
}

output "ids" {
  description = "Every tag this module created."
  value = {
    environment = uptimekuma_tag.environment.id
    team        = uptimekuma_tag.team.id
  }
}
