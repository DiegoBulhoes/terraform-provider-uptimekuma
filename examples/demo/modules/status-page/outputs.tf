output "slug" {
  description = "Slug of the page, which is also its Terraform ID."
  value       = uptimekuma_status_page.public.slug
}

output "page_id" {
  description = "Numeric ID of the page, which is what maintenance windows reference."
  value       = uptimekuma_status_page.public.page_id
}

output "incident_id" {
  description = "Composite ID of the pinned incident, as <slug>/<incident id>."
  value       = uptimekuma_status_page_incident.notice.id
}
