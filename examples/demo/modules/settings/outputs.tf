output "managed" {
  description = "The settings this module manages, as the server reports them."
  value       = jsondecode(uptimekuma_settings.instance.settings)
}
