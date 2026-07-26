data "uptimekuma_status_page" "public" {
  slug = "public"
}

# page_id is what a maintenance window needs.
resource "uptimekuma_maintenance" "window" {
  title       = "Weekly patching"
  description = "OS updates"
  strategy    = "manual"

  status_page_ids = [data.uptimekuma_status_page.public.page_id]
}

output "groups" {
  value = [for group in data.uptimekuma_status_page.public.groups : group.name]
}
