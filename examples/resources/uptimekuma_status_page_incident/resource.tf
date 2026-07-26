resource "uptimekuma_status_page" "public" {
  slug  = "public"
  title = "Example Status"
}

# To hide the banner but keep the incident in the page's history, set
# `pinned = false` instead of removing the resource.
resource "uptimekuma_status_page_incident" "degraded" {
  status_page_slug = uptimekuma_status_page.public.slug
  title            = "Degraded performance"
  content          = "We are investigating slow responses on the API. Updates to follow."
  style            = "warning"
}
